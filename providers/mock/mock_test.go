package mock

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/v2/core"
)

// newGateway mounts the handler under /mockpay like an app would, and returns a client that does not follow redirects.
func newGateway(t *testing.T) (*Provider, *http.Client, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	p, err := New(Config{BaseURL: server.URL + "/mockpay"})
	require.NoError(t, err)
	mux.Handle("/mockpay/", http.StripPrefix("/mockpay", p.Handler()))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return p, client, server
}

func initiate(t *testing.T, p *Provider, orderID string) *core.PaymentResponse {
	t.Helper()
	resp, err := p.InitiatePayment(context.Background(), &core.PaymentRequest{
		Amount: 10050, OrderID: orderID, CallbackURL: "https://shop.example/callback?src=mock",
	})
	require.NoError(t, err)
	return resp
}

func get(t *testing.T, client *http.Client, u string) (int, string) {
	t.Helper()
	resp, err := client.Get(u)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func post(t *testing.T, client *http.Client, server *httptest.Server, orderID, action string) *http.Response {
	t.Helper()
	resp, err := client.PostForm(server.URL+"/mockpay/pay", url.Values{"id": {orderID}, "action": {action}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestNew(t *testing.T) {
	_, err := New(Config{})
	assert.ErrorIs(t, err, core.ErrInvalidConfig)
}

func TestInitiatePayment(t *testing.T) {
	p, err := New(Config{BaseURL: "https://dev.example/mockpay"})
	require.NoError(t, err)

	tests := []struct {
		name    string
		req     *core.PaymentRequest
		wantErr error
	}{
		{"valid", &core.PaymentRequest{Amount: 100, OrderID: "A&B", CallbackURL: "https://x/cb"}, nil},
		{"duplicate order ID", &core.PaymentRequest{Amount: 100, OrderID: "A&B", CallbackURL: "https://x/cb"}, core.ErrInvalidRequest},
		{"zero amount", &core.PaymentRequest{OrderID: "B", CallbackURL: "https://x/cb"}, core.ErrInvalidRequest},
		{"missing order ID", &core.PaymentRequest{Amount: 100, CallbackURL: "https://x/cb"}, core.ErrInvalidRequest},
		{"missing callback", &core.PaymentRequest{Amount: 100, OrderID: "C"}, core.ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := p.InitiatePayment(context.Background(), tt.req)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.False(t, resp.IsFormPOST)
			assert.Equal(t, "https://dev.example/mockpay/pay?id=A%26B", resp.TargetURL)
			assert.Equal(t, "A&B", resp.TransactionID)
		})
	}
}

func TestFlow(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		wantRedir   bool
		wantStatus  string
		wantSuccess bool
	}{
		{"pay", "pay", true, core.StatusComplete, true},
		{"fail", "fail", true, core.StatusFailed, false},
		{"abandon", "abandon", false, core.StatusComplete, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, client, server := newGateway(t)
			started := initiate(t, p, "ORDER-1")

			code, body := get(t, client, started.TargetURL)
			require.Equal(t, http.StatusOK, code)
			assert.Contains(t, body, "ORDER-1")
			assert.Contains(t, body, "Rs. 100.50")

			// Not paid yet: a status check says pending, not an error.
			st, err := p.CheckStatus(context.Background(), &core.StatusRequest{OrderID: "ORDER-1", Amount: 10050})
			require.NoError(t, err)
			assert.Equal(t, core.StatusPending, st.Status)
			assert.False(t, st.Success)

			resp := post(t, client, server, "ORDER-1", tt.action)
			var verify *core.VerifyResponse
			if tt.wantRedir {
				require.Equal(t, http.StatusSeeOther, resp.StatusCode)
				loc, err := url.Parse(resp.Header.Get("Location"))
				require.NoError(t, err)
				assert.Equal(t, "shop.example", loc.Host)
				assert.Equal(t, "mock", loc.Query().Get("src"))
				params := map[string]string{"id": loc.Query().Get("id")}
				verify, err = p.VerifyPayment(context.Background(), &core.VerifyRequest{
					EncodedParams: params, OriginalOrderID: "ORDER-1", ExpectedAmount: 10050,
				})
				require.NoError(t, err)
			} else {
				require.Equal(t, http.StatusOK, resp.StatusCode)
				body, _ := io.ReadAll(resp.Body)
				assert.Contains(t, string(body), "close this tab")
				verify, err = p.CheckStatus(context.Background(), &core.StatusRequest{OrderID: "ORDER-1", Amount: 10050})
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStatus, verify.Status)
			assert.Equal(t, tt.wantSuccess, verify.Success)
			assert.Equal(t, "MOCK-ORDER-1", verify.TransactionID)
			assert.Equal(t, int64(10050), verify.VerifiedAmount)
			assert.Equal(t, "NPR", verify.Currency)
		})
	}
}

func TestFinishedPaymentCannotChange(t *testing.T) {
	p, client, server := newGateway(t)
	initiate(t, p, "ORDER-1")
	post(t, client, server, "ORDER-1", "fail")

	resp := post(t, client, server, "ORDER-1", "pay")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "already finished: FAILED")

	st, err := p.CheckStatus(context.Background(), &core.StatusRequest{OrderID: "ORDER-1"})
	require.NoError(t, err)
	assert.Equal(t, core.StatusFailed, st.Status)
}

func TestUnknownPayment(t *testing.T) {
	p, client, server := newGateway(t)
	code, _ := get(t, client, server.URL+"/mockpay/pay?id=nope")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, http.StatusNotFound, post(t, client, server, "nope", "pay").StatusCode)

	_, err := p.CheckStatus(context.Background(), &core.StatusRequest{OrderID: "nope"})
	assert.ErrorIs(t, err, core.ErrPaymentNotFound)
}

func TestVerifyPaymentChecks(t *testing.T) {
	p, _, _ := newGateway(t)
	initiate(t, p, "ORDER-1")

	tests := []struct {
		name    string
		req     *core.VerifyRequest
		wantErr error
	}{
		{"order ID mismatch", &core.VerifyRequest{EncodedParams: map[string]string{"id": "ORDER-1"}, OriginalOrderID: "ORDER-2"}, core.ErrInvalidRequest},
		{"unknown id", &core.VerifyRequest{EncodedParams: map[string]string{"id": "ORDER-9"}}, core.ErrPaymentNotFound},
		{"amount mismatch", &core.VerifyRequest{EncodedParams: map[string]string{"id": "ORDER-1"}, ExpectedAmount: 99}, core.ErrAmountMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.VerifyPayment(context.Background(), tt.req)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPageEscapesOrderID(t *testing.T) {
	p, client, _ := newGateway(t)
	started := initiate(t, p, "<script>alert(1)</script>")
	code, body := get(t, client, started.TargetURL)
	require.Equal(t, http.StatusOK, code)
	assert.NotContains(t, body, "<script>")
	assert.True(t, strings.Contains(body, "&lt;script&gt;"))
}
