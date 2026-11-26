package payments_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/v2/core"
	"github.com/voidarchive/nepal-payment-go/v2/payments"
	"github.com/voidarchive/nepal-payment-go/v2/providers/mock"
)

var (
	ctx   = context.Background()
	start = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

// checker is a stub gateway that also implements core.StatusChecker.
type checker struct {
	core.MockProvider
	check func(*core.StatusRequest) (*core.VerifyResponse, error)
}

func (c *checker) CheckStatus(_ context.Context, r *core.StatusRequest) (*core.VerifyResponse, error) {
	return c.check(r)
}

type env struct {
	svc    *payments.Service
	now    time.Time // the service's clock; advance it between calls
	server *httptest.Server
	client *http.Client
}

// newEnv wires a Service to the mock gateway ("mock") and any extra providers.
func newEnv(t *testing.T, extra map[string]core.Provider) *env {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	gw, err := mock.New(mock.Config{BaseURL: server.URL + "/mockpay"})
	require.NoError(t, err)
	mux.Handle("/mockpay/", http.StripPrefix("/mockpay", gw.Handler()))

	e := &env{
		now:    start,
		server: server,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	providers := map[string]core.Provider{"mock": gw}
	for k, v := range extra {
		providers[k] = v
	}
	e.svc = &payments.Service{
		Store:       payments.NewMemoryStore(),
		Providers:   providers,
		CallbackURL: func(id string) string { return "https://shop.example/callback/" + id },
		Now:         func() time.Time { return e.now },
	}
	return e
}

func (e *env) create(t *testing.T, orderID, provider string, amount int64) *payments.Payment {
	t.Helper()
	p, err := e.svc.Create(ctx, payments.CreateParams{OrderID: orderID, Provider: provider, Amount: amount})
	require.NoError(t, err)
	return p
}

// pay acts as the customer on the mock gateway's page and returns the callback parameters (nil for "abandon").
func (e *env) pay(t *testing.T, id, action string) map[string]string {
	t.Helper()
	resp, err := e.client.PostForm(e.server.URL+"/mockpay/pay", url.Values{"id": {id}, "action": {action}})
	require.NoError(t, err)
	_ = resp.Body.Close()
	if action == "abandon" {
		require.Equal(t, http.StatusOK, resp.StatusCode)
		return nil
	}
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	loc, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/callback/"+id, loc.Path)
	params := map[string]string{}
	for k, v := range loc.Query() {
		params[k] = v[0]
	}
	return params
}

func (e *env) get(t *testing.T, id string) *payments.Payment {
	t.Helper()
	p, err := e.svc.Store.Get(ctx, id)
	require.NoError(t, err)
	return p
}

func TestFullFlowAndRepeatCallback(t *testing.T) {
	e := newEnv(t, nil)
	p := e.create(t, "order-1", "mock", 10050)
	assert.Equal(t, payments.StatusPending, p.Status)
	assert.Equal(t, p.ID, p.ProviderRef)
	assert.Contains(t, p.Checkout.TargetURL, "/mockpay/pay?id="+p.ID)
	assert.Equal(t, start.Add(time.Minute), p.NextCheckAt)

	params := e.pay(t, p.ID, "pay")
	got, changed, err := e.svc.HandleCallback(ctx, p.ID, params)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, payments.StatusPaid, got.Status)
	assert.Equal(t, core.StatusComplete, got.ProviderStatus)
	assert.Equal(t, "MOCK-"+p.ID, got.TransactionID)
	assert.Equal(t, payments.StatusPaid, e.get(t, p.ID).Status)

	got, changed, err = e.svc.HandleCallback(ctx, p.ID, params)
	require.NoError(t, err)
	assert.False(t, changed, "a second callback is not a change")
	assert.Equal(t, payments.StatusPaid, got.Status)
}

func TestFailedPaymentCallback(t *testing.T) {
	e := newEnv(t, nil)
	p := e.create(t, "order-1", "mock", 10050)
	got, changed, err := e.svc.HandleCallback(ctx, p.ID, e.pay(t, p.ID, "fail"))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, payments.StatusFailed, got.Status)
}

func TestCallbackAndCheckRace(t *testing.T) {
	e := newEnv(t, nil)
	for i := range 20 {
		p := e.create(t, fmt.Sprint("order-", i), "mock", 500)
		params := e.pay(t, p.ID, "pay")

		var mu sync.Mutex
		changes := 0
		var wg sync.WaitGroup
		for j := range 4 {
			wg.Go(func() {
				var changed bool
				var err error
				if j%2 == 0 {
					_, changed, err = e.svc.HandleCallback(ctx, p.ID, params)
				} else {
					_, changed, err = e.svc.Check(ctx, p.ID)
				}
				assert.NoError(t, err)
				mu.Lock()
				defer mu.Unlock()
				if changed {
					changes++
				}
			})
		}
		wg.Wait()
		assert.Equal(t, 1, changes)
		assert.Equal(t, payments.StatusPaid, e.get(t, p.ID).Status)
	}
}

func TestCheckDueFindsAbandonedPayment(t *testing.T) {
	e := newEnv(t, nil)
	p := e.create(t, "order-1", "mock", 10050)
	e.pay(t, p.ID, "abandon")

	changed, err := e.svc.CheckDue(ctx)
	require.NoError(t, err)
	assert.Empty(t, changed, "not due yet")

	e.now = e.now.Add(time.Minute)
	changed, err = e.svc.CheckDue(ctx)
	require.NoError(t, err)
	require.Len(t, changed, 1)
	assert.Equal(t, p.ID, changed[0].ID)
	assert.Equal(t, payments.StatusPaid, changed[0].Status)

	changed, err = e.svc.CheckDue(ctx)
	require.NoError(t, err)
	assert.Empty(t, changed)
}

func TestBackoffSchedule(t *testing.T) {
	e := newEnv(t, nil)
	p := e.create(t, "order-1", "mock", 10050)
	for i, wantDelay := range []time.Duration{2, 4, 8, 16, 32, 60, 60, 60} {
		e.now = e.get(t, p.ID).NextCheckAt
		got, changed, err := e.svc.Check(ctx, p.ID)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, payments.StatusPending, got.Status)
		assert.Equal(t, i+1, got.Checks)
		assert.Equal(t, e.now.Add(wantDelay*time.Minute), got.NextCheckAt, "check %d", i+1)
		assert.Equal(t, got, e.get(t, p.ID))
	}
}

func TestExpiry(t *testing.T) {
	uncheckable := &core.MockProvider{}
	e := newEnv(t, map[string]core.Provider{"plain": uncheckable})

	t.Run("checkable provider after 24h", func(t *testing.T) {
		p := e.create(t, "order-1", "mock", 10050)
		e.now = p.CreatedAt.Add(24*time.Hour - time.Second)
		got, changed, err := e.svc.Check(ctx, p.ID)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, payments.StatusPending, got.Status)

		e.now = p.CreatedAt.Add(24 * time.Hour)
		got, changed, err = e.svc.Check(ctx, p.ID)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, payments.StatusExpired, got.Status)
		assert.Equal(t, payments.StatusExpired, e.get(t, p.ID).Status)
	})

	t.Run("provider without StatusChecker after 30m", func(t *testing.T) {
		e.now = start
		p := e.create(t, "order-2", "plain", 10050)
		e.now = p.NextCheckAt
		changed, err := e.svc.CheckDue(ctx)
		require.NoError(t, err)
		assert.Empty(t, changed)
		got := e.get(t, p.ID)
		assert.Equal(t, payments.StatusPending, got.Status)
		assert.Zero(t, got.Checks, "never checked")
		assert.Equal(t, p.CreatedAt.Add(30*time.Minute), got.NextCheckAt)

		e.now = got.NextCheckAt
		changed, err = e.svc.CheckDue(ctx)
		require.NoError(t, err)
		require.Len(t, changed, 1)
		assert.Equal(t, payments.StatusExpired, changed[0].Status)
		assert.Zero(t, changed[0].Checks)
	})
}

func TestCheckStatusErrorAdvancesSchedule(t *testing.T) {
	broken := &checker{check: func(*core.StatusRequest) (*core.VerifyResponse, error) {
		return nil, core.ErrProviderError
	}}
	e := newEnv(t, map[string]core.Provider{"broken": broken})
	p := e.create(t, "order-1", "broken", 10050)

	e.now = p.NextCheckAt
	changed, err := e.svc.CheckDue(ctx)
	require.ErrorIs(t, err, core.ErrProviderError)
	assert.Empty(t, changed)
	got := e.get(t, p.ID)
	assert.Equal(t, payments.StatusPending, got.Status)
	assert.Equal(t, 1, got.Checks)
	assert.Equal(t, e.now.Add(2*time.Minute), got.NextCheckAt)

	changed, err = e.svc.CheckDue(ctx)
	require.NoError(t, err, "not due again yet")
	assert.Empty(t, changed)
}

func TestCreate(t *testing.T) {
	e := newEnv(t, map[string]core.Provider{"other": &core.MockProvider{}})
	first := e.create(t, "order-1", "mock", 10050)

	t.Run("ID format", func(t *testing.T) {
		assert.Regexp(t, regexp.MustCompile(`^[a-z0-9]{20}$`), first.ID)
	})
	t.Run("repeat returns the same payment", func(t *testing.T) {
		again := e.create(t, "order-1", "mock", 10050)
		assert.Equal(t, first.ID, again.ID)
	})
	t.Run("amount conflict", func(t *testing.T) {
		_, err := e.svc.Create(ctx, payments.CreateParams{OrderID: "order-1", Provider: "mock", Amount: 1})
		assert.ErrorIs(t, err, payments.ErrAmountConflict)
	})
	t.Run("other provider gives a new payment", func(t *testing.T) {
		other := e.create(t, "order-1", "other", 10050)
		assert.NotEqual(t, first.ID, other.ID)
		assert.Equal(t, "other", other.Provider)
	})
	t.Run("retry after a failed payment gives a new ID", func(t *testing.T) {
		p := e.create(t, "order-2", "mock", 10050)
		_, _, err := e.svc.HandleCallback(ctx, p.ID, e.pay(t, p.ID, "fail"))
		require.NoError(t, err)
		retry := e.create(t, "order-2", "mock", 10050)
		assert.NotEqual(t, p.ID, retry.ID)
		assert.Equal(t, payments.StatusPending, retry.Status)
	})
	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			name string
			in   payments.CreateParams
			want error
		}{
			{"unknown provider", payments.CreateParams{OrderID: "o", Provider: "nope", Amount: 1}, payments.ErrUnknownProvider},
			{"no order ID", payments.CreateParams{Provider: "mock", Amount: 1}, core.ErrInvalidRequest},
			{"zero amount", payments.CreateParams{OrderID: "o", Provider: "mock"}, core.ErrInvalidRequest},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := e.svc.Create(ctx, tt.in)
				assert.ErrorIs(t, err, tt.want)
			})
		}
	})
	t.Run("gateway error stores nothing", func(t *testing.T) {
		down := &core.MockProvider{InitiateFunc: func(context.Context, *core.PaymentRequest) (*core.PaymentResponse, error) {
			return nil, core.ErrProviderError
		}}
		e.svc.Providers["down"] = down
		_, err := e.svc.Create(ctx, payments.CreateParams{OrderID: "order-3", Provider: "down", Amount: 1})
		assert.ErrorIs(t, err, core.ErrProviderError)
		e.svc.Providers["down"] = &core.MockProvider{}
		p := e.create(t, "order-3", "down", 2) // a different amount is fine: nothing was stored
		assert.Equal(t, int64(2), p.Amount)
	})
}

func TestRepeatCreateSkipsGateway(t *testing.T) {
	calls := 0
	gw := &core.MockProvider{InitiateFunc: func(context.Context, *core.PaymentRequest) (*core.PaymentResponse, error) {
		calls++
		if calls > 1 {
			return nil, core.ErrProviderError // the gateway is down for the repeat
		}
		return &core.PaymentResponse{}, nil
	}}
	e := newEnv(t, map[string]core.Provider{"gw": gw})
	first := e.create(t, "order-1", "gw", 10050)

	again := e.create(t, "order-1", "gw", 10050)
	assert.Equal(t, first.ID, again.ID)
	_, err := e.svc.Create(ctx, payments.CreateParams{OrderID: "order-1", Provider: "gw", Amount: 1})
	assert.ErrorIs(t, err, payments.ErrAmountConflict)
	assert.Equal(t, 1, calls)
}

func TestVerifyErrorLeavesPending(t *testing.T) {
	var got *core.VerifyRequest
	bad := &core.MockProvider{VerifyFunc: func(_ context.Context, r *core.VerifyRequest) (*core.VerifyResponse, error) {
		got = r
		return nil, core.ErrSignatureMismatch
	}}
	bad.InitiateFunc = func(context.Context, *core.PaymentRequest) (*core.PaymentResponse, error) {
		return &core.PaymentResponse{TransactionID: "pidx-1"}, nil
	}
	e := newEnv(t, map[string]core.Provider{"bad": bad})
	p := e.create(t, "order-1", "bad", 10050)

	_, changed, err := e.svc.HandleCallback(ctx, p.ID, map[string]string{"data": "forged"})
	require.ErrorIs(t, err, core.ErrSignatureMismatch)
	assert.False(t, changed)
	assert.Equal(t, payments.StatusPending, e.get(t, p.ID).Status)
	assert.Equal(t, &core.VerifyRequest{
		EncodedParams: map[string]string{"data": "forged"}, OriginalOrderID: p.ID, ExpectedAmount: 10050, ProviderRef: "pidx-1",
	}, got)
}

func TestCallbackWithOtherPaymentsIDIsRejected(t *testing.T) {
	e := newEnv(t, nil)
	a := e.create(t, "order-a", "mock", 10050)
	b := e.create(t, "order-b", "mock", 10050)
	paramsB := e.pay(t, b.ID, "pay")

	_, changed, err := e.svc.HandleCallback(ctx, a.ID, paramsB)
	require.ErrorIs(t, err, core.ErrInvalidRequest)
	assert.False(t, changed)
	assert.Equal(t, payments.StatusPending, e.get(t, a.ID).Status)
}

func TestCheckSeesRefund(t *testing.T) {
	status := core.StatusComplete
	gw := &checker{check: func(*core.StatusRequest) (*core.VerifyResponse, error) {
		return &core.VerifyResponse{Status: status, TransactionID: "tx-1"}, nil
	}}
	gw.VerifyFunc = func(context.Context, *core.VerifyRequest) (*core.VerifyResponse, error) {
		return &core.VerifyResponse{Success: true, Status: core.StatusComplete, TransactionID: "tx-1"}, nil
	}
	e := newEnv(t, map[string]core.Provider{"gw": gw})
	p := e.create(t, "order-1", "gw", 10050)
	_, changed, err := e.svc.HandleCallback(ctx, p.ID, nil)
	require.NoError(t, err)
	require.True(t, changed)

	got, changed, err := e.svc.Check(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, changed, "still paid")
	assert.Equal(t, payments.StatusPaid, got.Status)

	status = core.StatusRefunded
	got, changed, err = e.svc.Check(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, payments.StatusRefunded, got.Status)
	assert.Equal(t, core.StatusRefunded, e.get(t, p.ID).ProviderStatus)

	got, changed, err = e.svc.Check(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, changed, "refunded is final")
	assert.Equal(t, payments.StatusRefunded, got.Status)
}

func TestCheckExpiresPaymentOfRemovedProvider(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := payments.NewMemoryStore()
	_, err := store.Create(context.Background(), &payments.Payment{ID: "p1", OrderID: "o1", Provider: "gone", Amount: 100, Status: payments.StatusPending, CreatedAt: now})
	require.NoError(t, err)
	s := &payments.Service{Store: store, Providers: map[string]core.Provider{}, Now: func() time.Time { return now.Add(31 * time.Minute) }}

	changed, err := s.CheckDue(context.Background())
	require.ErrorIs(t, err, payments.ErrUnknownProvider)
	require.Len(t, changed, 1)
	assert.Equal(t, payments.StatusExpired, changed[0].Status)
}
