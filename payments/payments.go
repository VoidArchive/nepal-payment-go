// Package payments owns payment records on top of the stateless providers:
// it creates payments, applies gateway callbacks and polls gateways for
// payments whose customer never came back.
package payments

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voidarchive/nepal-payment-go/v2/core"
)

// Status is the lifecycle state of a Payment.
type Status string

// Payment statuses. Moves allowed: pending -> paid | failed | expired | refunded, paid -> refunded.
const (
	StatusPending  Status = "pending"
	StatusPaid     Status = "paid"
	StatusFailed   Status = "failed"
	StatusExpired  Status = "expired"
	StatusRefunded Status = "refunded"
)

var (
	ErrNotFound        = errors.New("payments: payment not found")
	ErrAmountConflict  = errors.New("payments: order already has a payment with a different amount")
	ErrUnknownProvider = errors.New("payments: unknown provider")
)

// Payment is one attempt to pay an order through one provider.
type Payment struct {
	ID             string // made by this package; also sent to the gateway as its order ID
	OrderID        string // the application's own order ID
	Provider       string // key in Service.Providers
	Amount         int64  // paisa
	Status         Status
	ProviderStatus string               // last status the gateway reported (core.Status* value), for display
	ProviderRef    string               // PaymentResponse.TransactionID from InitiatePayment (Khalti pidx)
	TransactionID  string               // the gateway's transaction ID, set when known
	Checkout       core.PaymentResponse // where to send the customer
	Checks         int                  // status checks done so far

	CreatedAt, UpdatedAt, NextCheckAt time.Time
}

// Store persists payments. Implementations must be safe for concurrent use;
// storetest.Run checks the contract.
type Store interface {
	// Create inserts p, unless the order already has an active payment:
	// a pending payment with the same OrderID AND Provider, or a paid payment
	// with the same OrderID (any provider). Then it inserts nothing and returns
	// that payment. The check and the insert are atomic.
	Create(ctx context.Context, p *Payment) (existing *Payment, err error)
	// Get returns ErrNotFound for an unknown ID.
	Get(ctx context.Context, id string) (*Payment, error)
	// Update writes p only if the stored status is still `from`. ok is false
	// when another caller changed it first (nothing is written then).
	Update(ctx context.Context, p *Payment, from Status) (ok bool, err error)
	// ListDue returns pending payments with NextCheckAt <= now, oldest first.
	ListDue(ctx context.Context, now time.Time, limit int) ([]*Payment, error)
}

// ActiveFinder is an optional interface a Store may also implement. FindActive
// returns the order's active payment by the same rule as Store.Create (a paid
// payment with the same OrderID, any provider, or a pending payment with the
// same OrderID AND provider), or (nil, nil) when there is none. With it,
// Service.Create answers a repeat without calling the gateway.
type ActiveFinder interface {
	FindActive(ctx context.Context, orderID, provider string) (*Payment, error)
}

// Service creates payments and moves them through their statuses.
type Service struct {
	Store             Store
	Providers         map[string]core.Provider
	CallbackURL       func(paymentID string) string // the URL the gateway sends the customer back to
	Now               func() time.Time              // nil means time.Now
	Expiry            time.Duration                 // pending payments expire after this; 0 means 24h
	UncheckableExpiry time.Duration                 // same, for providers without core.StatusChecker; 0 means 30m
}

// CreateParams describes a payment to create.
type CreateParams struct {
	OrderID, Provider                         string
	Amount                                    int64 // paisa
	Description, CustomerEmail, CustomerPhone string
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) provider(name string) (core.Provider, error) {
	prov, ok := s.Providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, name)
	}
	return prov, nil
}

// newID returns 20 characters from [a-z2-7]: ConnectIPS allows 20, eSewa only alphanumerics and hyphens.
func newID() string { return strings.ToLower(rand.Text()[:20]) }

// Create starts a payment for an order. A repeat for an order that already
// has an active payment (see Store.Create) returns that payment instead.
func (s *Service) Create(ctx context.Context, in CreateParams) (*Payment, error) {
	if in.OrderID == "" {
		return nil, fmt.Errorf("%w: OrderID is required", core.ErrInvalidRequest)
	}
	if in.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", core.ErrInvalidRequest)
	}
	prov, err := s.provider(in.Provider)
	if err != nil {
		return nil, err
	}

	if f, ok := s.Store.(ActiveFinder); ok {
		existing, err := f.FindActive(ctx, in.OrderID, in.Provider)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if existing.Amount != in.Amount {
				return nil, ErrAmountConflict
			}
			return existing, nil
		}
	}

	id := newID()
	// Initiate before storing so a gateway failure leaves no half-written record.
	// A repeat that FindActive did not catch (no ActiveFinder, or a concurrent
	// first request) is caught by Store.Create and the fresh gateway session is dropped.
	resp, err := prov.InitiatePayment(ctx, &core.PaymentRequest{
		Amount:        in.Amount,
		OrderID:       id,
		Description:   in.Description,
		CallbackURL:   s.CallbackURL(id),
		CustomerEmail: in.CustomerEmail,
		CustomerPhone: in.CustomerPhone,
	})
	if err != nil {
		return nil, err
	}

	now := s.now()
	p := &Payment{
		ID:          id,
		OrderID:     in.OrderID,
		Provider:    in.Provider,
		Amount:      in.Amount,
		Status:      StatusPending,
		ProviderRef: resp.TransactionID,
		Checkout:    *resp,
		CreatedAt:   now,
		UpdatedAt:   now,
		NextCheckAt: now.Add(time.Minute),
	}
	existing, err := s.Store.Create(ctx, p)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Amount != in.Amount {
			return nil, ErrAmountConflict
		}
		return existing, nil
	}
	return p, nil
}

// HandleCallback verifies the gateway's callback for payment id. changed is
// true for exactly one caller per status transition.
func (s *Service) HandleCallback(ctx context.Context, id string, params map[string]string) (*Payment, bool, error) {
	p, err := s.Store.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if p.Status != StatusPending {
		return p, false, nil // callbacks can arrive twice
	}
	prov, err := s.provider(p.Provider)
	if err != nil {
		return nil, false, err
	}
	r, err := prov.VerifyPayment(ctx, &core.VerifyRequest{
		EncodedParams:   params,
		OriginalOrderID: p.ID,
		ExpectedAmount:  p.Amount,
		ProviderRef:     p.ProviderRef,
	})
	if err != nil {
		return p, false, err // stays pending; Check settles it later
	}
	if !settle(p, r) {
		return p, false, nil
	}
	return s.save(ctx, p, StatusPending)
}

// Check asks the gateway for the status of a pending or paid payment (paid,
// to notice a refund), expires stale pending payments and schedules the next check.
func (s *Service) Check(ctx context.Context, id string) (*Payment, bool, error) {
	p, err := s.Store.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if p.Status != StatusPending && p.Status != StatusPaid {
		return p, false, nil
	}
	// A provider removed from the configuration is treated as uncheckable, so its
	// pending payments still expire and do not stay due forever.
	prov, checkErr := s.provider(p.Provider)
	from := p.Status

	checker, checkable := prov.(core.StatusChecker)
	if checkable {
		r, err := checker.CheckStatus(ctx, &core.StatusRequest{OrderID: p.ID, ProviderRef: p.ProviderRef, Amount: p.Amount})
		switch {
		case err == nil:
			if settle(p, r) {
				return s.save(ctx, p, from)
			}
		case !errors.Is(err, core.ErrPaymentNotFound): // not found: the customer never reached the gateway
			checkErr = err
		}
	}
	if p.Status != StatusPending {
		return p, false, checkErr
	}

	now := s.now()
	expiry := cmp.Or(s.Expiry, 24*time.Hour)
	if !checkable {
		expiry = cmp.Or(s.UncheckableExpiry, 30*time.Minute)
	}
	if now.Sub(p.CreatedAt) >= expiry {
		p.Status = StatusExpired
		p, changed, err := s.save(ctx, p, from)
		return p, changed, errors.Join(checkErr, err)
	}
	if checkable {
		p.Checks++
		p.NextCheckAt = now.Add(min(time.Minute<<min(p.Checks, 6), time.Hour))
	} else {
		p.NextCheckAt = p.CreatedAt.Add(expiry) // nothing to ask the gateway; wake up only to expire it
	}
	p, _, err = s.save(ctx, p, from) // a schedule update is not a change
	return p, false, errors.Join(checkErr, err)
}

// CheckDue runs Check on up to 100 due payments and returns those that changed.
// It keeps going after an error and returns all errors joined. Run it on a timer.
func (s *Service) CheckDue(ctx context.Context) ([]*Payment, error) {
	due, err := s.Store.ListDue(ctx, s.now(), 100)
	if err != nil {
		return nil, err
	}
	var changed []*Payment
	var errs []error
	for _, d := range due {
		p, ok, err := s.Check(ctx, d.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("payment %s: %w", d.ID, err))
		}
		if ok {
			changed = append(changed, p)
		}
	}
	return changed, errors.Join(errs...)
}

// settle applies a gateway result to p and reports whether its status moved.
func settle(p *Payment, r *core.VerifyResponse) bool {
	p.ProviderStatus = r.Status
	if r.TransactionID != "" {
		p.TransactionID = r.TransactionID
	}
	var to Status
	switch r.Status {
	case core.StatusComplete:
		to = StatusPaid
	case core.StatusRefunded:
		to = StatusRefunded
	case core.StatusFailed, core.StatusCancelled:
		to = StatusFailed
	case core.StatusExpired:
		to = StatusExpired
	default: // PENDING or unknown
		return false
	}
	if p.Status == StatusPending || p.Status == StatusPaid && to == StatusRefunded {
		p.Status = to
		return true
	}
	return false
}

// save writes p if its stored status is still from. When another caller got
// there first it returns the stored payment and false.
func (s *Service) save(ctx context.Context, p *Payment, from Status) (*Payment, bool, error) {
	p.UpdatedAt = s.now()
	ok, err := s.Store.Update(ctx, p, from)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		cur, err := s.Store.Get(ctx, p.ID)
		return cur, false, err
	}
	return p, true, nil
}
