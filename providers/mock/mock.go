// Package mock is a fake payment gateway for development and tests only.
//
// It behaves like a real gateway: the customer decides the result on the
// gateway's own page (served by Handler), and VerifyPayment reads the
// gateway's record, never the callback parameters. No merchant account needed.
package mock

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/voidarchive/nepal-payment-go/v2/core"
)

// Config holds mock provider configuration.
type Config struct {
	BaseURL string // public URL where Handler() is mounted, e.g. "http://localhost:8080/mockpay"
}

type payment struct {
	amount      int64
	callbackURL string
	status      string // core.StatusPending, StatusComplete or StatusFailed
}

// Provider is an in-memory fake gateway. It implements core.Provider and core.StatusChecker.
type Provider struct {
	baseURL string
	mu      sync.Mutex
	// shortcut: payments live in memory and are lost on restart; fine for development, never use in production
	payments map[string]*payment
}

// New creates a mock provider.
func New(cfg Config) (*Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("%w: BaseURL is required", core.ErrInvalidConfig)
	}
	return &Provider{baseURL: cfg.BaseURL, payments: map[string]*payment{}}, nil
}

// InitiatePayment records a pending payment and returns the URL of its payment page.
func (p *Provider) InitiatePayment(ctx context.Context, req *core.PaymentRequest) (*core.PaymentResponse, error) {
	if req.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", core.ErrInvalidRequest)
	}
	if req.OrderID == "" {
		return nil, fmt.Errorf("%w: OrderID is required", core.ErrInvalidRequest)
	}
	if req.CallbackURL == "" {
		return nil, fmt.Errorf("%w: CallbackURL is required", core.ErrInvalidRequest)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.payments[req.OrderID]; ok {
		return nil, fmt.Errorf("%w: duplicate OrderID %s", core.ErrInvalidRequest, req.OrderID)
	}
	p.payments[req.OrderID] = &payment{amount: req.Amount, callbackURL: req.CallbackURL, status: core.StatusPending}

	return &core.PaymentResponse{
		IsFormPOST:    false,
		TargetURL:     p.baseURL + "/pay?id=" + url.QueryEscape(req.OrderID),
		TransactionID: req.OrderID,
	}, nil
}

// VerifyPayment reads the stored payment named by EncodedParams["id"].
func (p *Provider) VerifyPayment(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	id := req.EncodedParams["id"]
	if req.OriginalOrderID != "" && req.OriginalOrderID != id {
		return nil, fmt.Errorf("%w: order ID mismatch: expected %s, got %s", core.ErrInvalidRequest, req.OriginalOrderID, id)
	}
	return p.status(id, req.ExpectedAmount)
}

// CheckStatus reads the stored payment without callback data.
func (p *Provider) CheckStatus(ctx context.Context, req *core.StatusRequest) (*core.VerifyResponse, error) {
	return p.status(req.OrderID, req.Amount)
}

func (p *Provider) status(orderID string, expectedAmount int64) (*core.VerifyResponse, error) {
	p.mu.Lock()
	pay, ok := p.payments[orderID]
	var amount int64
	var status string
	if ok {
		amount, status = pay.amount, pay.status
	}
	p.mu.Unlock()

	if !ok {
		return nil, core.ErrPaymentNotFound
	}
	if expectedAmount > 0 && expectedAmount != amount {
		return nil, fmt.Errorf("%w: expected %d, got %d", core.ErrAmountMismatch, expectedAmount, amount)
	}
	return &core.VerifyResponse{
		Success:          status == core.StatusComplete,
		TransactionID:    "MOCK-" + orderID,
		OrderID:          orderID,
		VerifiedAmount:   amount,
		Currency:         "NPR",
		Status:           status,
		ProviderResponse: map[string]string{"status": status},
		VerifiedAt:       time.Now().Unix(),
	}, nil
}

var page = template.Must(template.New("pay").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mock payment gateway</title></head>
<body style="font-family:sans-serif;max-width:32rem;margin:2rem auto;padding:0 1rem">
<h1>Mock payment gateway</h1>
<p>Order: <strong>{{.ID}}</strong><br>Amount: <strong>Rs. {{.Rupees}}</strong></p>
{{if .Message}}<p>{{.Message}}</p>
{{else}}<form method="post" action="pay">
<input type="hidden" name="id" value="{{.ID}}">
<button name="action" value="pay">Pay</button>
<button name="action" value="fail">Fail</button>
<button name="action" value="abandon">Pay, then close the tab</button>
</form>{{end}}
</body></html>`))

// Handler serves the gateway's payment page. Mount it at Config.BaseURL; it works under http.StripPrefix.
func (p *Provider) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/pay", p.servePay)
	return mux
}

func (p *Provider) servePay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("id")

	p.mu.Lock()
	pay, ok := p.payments[id]
	if !ok {
		p.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	amount, callbackURL := pay.amount, pay.callbackURL
	message := ""
	action := r.PostFormValue("action")
	switch {
	case pay.status != core.StatusPending:
		message = "This payment is already finished: " + pay.status + "."
	case r.Method == http.MethodGet:
	case action == "pay" || action == "abandon":
		pay.status = core.StatusComplete
	case action == "fail":
		pay.status = core.StatusFailed
	default:
		p.mu.Unlock()
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	p.mu.Unlock()

	if message == "" && (action == "pay" || action == "fail") {
		u, err := url.Parse(callbackURL)
		if err != nil {
			http.Error(w, "bad callback URL", http.StatusInternalServerError)
			return
		}
		q := u.Query()
		q.Set("id", id)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusSeeOther)
		return
	}
	if message == "" && action == "abandon" {
		message = "Payment recorded. You can close this tab."
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, map[string]string{
		"ID":      id,
		"Rupees":  strconv.FormatFloat(float64(amount)/100, 'f', 2, 64),
		"Message": message,
	})
}

var (
	_ core.Provider      = (*Provider)(nil)
	_ core.StatusChecker = (*Provider)(nil)
)
