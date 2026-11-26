package khalti

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/v2/core"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
		wantNil bool
	}{
		{
			name: "valid config",
			config: Config{
				SecretKey:  "test_secret",
				WebsiteURL: "https://example.com",
				Sandbox:    true,
			},
			wantErr: nil,
			wantNil: false,
		},
		{
			name: "missing secret key",
			config: Config{
				WebsiteURL: "https://example.com",
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "missing website URL",
			config: Config{
				SecretKey: "test_secret",
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "custom base URL",
			config: Config{
				SecretKey:  "test_secret",
				WebsiteURL: "https://example.com",
				BaseURL:    "https://custom.api.com",
			},
			wantErr: nil,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := New(tt.config)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tt.wantNil {
				assert.Nil(t, provider)
			} else {
				assert.NotNil(t, provider)
			}
		})
	}
}

func TestInitiatePayment(t *testing.T) {
	tests := []struct {
		name           string
		request        *core.PaymentRequest
		serverResponse *initiateResponse
		serverStatus   int
		wantErr        error
		wantTargetURL  string
		wantTxnID      string
	}{
		{
			name: "successful initiation",
			request: &core.PaymentRequest{
				Amount:      10000,
				Currency:    "NPR",
				OrderID:     "ORDER-123",
				Description: "Test Order",
				CallbackURL: "https://example.com/callback",
			},
			serverResponse: &initiateResponse{
				PIDX:       "test_pidx_123",
				PaymentURL: "https://pay.khalti.com/?pidx=test_pidx_123",
			},
			serverStatus:  http.StatusOK,
			wantErr:       nil,
			wantTargetURL: "https://pay.khalti.com/?pidx=test_pidx_123",
			wantTxnID:     "test_pidx_123",
		},
		{
			name: "with customer info",
			request: &core.PaymentRequest{
				Amount:        10000,
				OrderID:       "ORDER-456",
				CallbackURL:   "https://example.com/callback",
				CustomerEmail: "test@example.com",
				CustomerPhone: "9800000000",
			},
			serverResponse: &initiateResponse{
				PIDX:       "pidx_with_customer",
				PaymentURL: "https://pay.khalti.com/?pidx=pidx_with_customer",
			},
			serverStatus:  http.StatusOK,
			wantErr:       nil,
			wantTargetURL: "https://pay.khalti.com/?pidx=pidx_with_customer",
			wantTxnID:     "pidx_with_customer",
		},
		{
			name: "zero amount",
			request: &core.PaymentRequest{
				Amount:      0,
				OrderID:     "ORDER-123",
				CallbackURL: "https://example.com/callback",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "negative amount",
			request: &core.PaymentRequest{
				Amount:      -100,
				OrderID:     "ORDER-123",
				CallbackURL: "https://example.com/callback",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "missing order ID",
			request: &core.PaymentRequest{
				Amount:      10000,
				CallbackURL: "https://example.com/callback",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "missing callback URL",
			request: &core.PaymentRequest{
				Amount:  10000,
				OrderID: "ORDER-123",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "rate limited",
			request: &core.PaymentRequest{
				Amount:      10000,
				OrderID:     "ORDER-123",
				CallbackURL: "https://example.com/callback",
			},
			serverStatus: http.StatusTooManyRequests,
			wantErr:      core.ErrRateLimited,
		},
		{
			name: "unauthorized",
			request: &core.PaymentRequest{
				Amount:      10000,
				OrderID:     "ORDER-123",
				CallbackURL: "https://example.com/callback",
			},
			serverStatus: http.StatusUnauthorized,
			wantErr:      core.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server

			// Only create server if we need to test API interaction
			if tt.serverResponse != nil || tt.serverStatus != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, initiateEndpoint, r.URL.Path)
					assert.Equal(t, "Key test_secret", r.Header.Get("Authorization"))

					if tt.serverStatus != 0 && tt.serverStatus != http.StatusOK {
						w.WriteHeader(tt.serverStatus)
						return
					}

					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.serverResponse) //nolint:errcheck
				}))
				defer server.Close()
			}

			cfg := Config{
				SecretKey:  "test_secret",
				WebsiteURL: "https://example.com",
			}
			if server != nil {
				cfg.BaseURL = server.URL
			}

			provider, err := New(cfg)
			require.NoError(t, err)

			resp, err := provider.InitiatePayment(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.False(t, resp.IsFormPOST, "Khalti should use redirect, not form POST")
				assert.Equal(t, tt.wantTargetURL, resp.TargetURL)
				assert.Equal(t, tt.wantTxnID, resp.TransactionID)
			}
		})
	}
}

func TestVerifyPayment(t *testing.T) {
	tests := []struct {
		name           string
		request        *core.VerifyRequest
		serverResponse *lookupResponse
		serverStatus   int
		wantErr        error
		wantSuccess    bool
		wantStatus     string
		wantAmount     int64
		wantTxnID      string
	}{
		{
			name: "completed payment",
			request: &core.VerifyRequest{
				EncodedParams:   map[string]string{"pidx": "test_pidx"},
				OriginalOrderID: "ORDER-123",
				ExpectedAmount:  10000,
			},
			serverResponse: &lookupResponse{
				PIDX:          "test_pidx",
				TotalAmount:   10000,
				Status:        "Completed",
				TransactionID: "txn_abc123",
			},
			serverStatus: http.StatusOK,
			wantErr:      nil,
			wantSuccess:  true,
			wantStatus:   core.StatusComplete,
			wantAmount:   10000,
			wantTxnID:    "txn_abc123",
		},
		{
			name: "pending payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{"pidx": "test_pidx"},
			},
			serverResponse: &lookupResponse{
				PIDX:        "test_pidx",
				TotalAmount: 5000,
				Status:      "Pending",
			},
			serverStatus: http.StatusOK,
			wantErr:      nil,
			wantSuccess:  false,
			wantStatus:   core.StatusPending,
			wantAmount:   5000,
		},
		{
			name: "amount mismatch",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"pidx": "test_pidx"},
				ExpectedAmount: 10000,
			},
			serverResponse: &lookupResponse{
				PIDX:          "test_pidx",
				TotalAmount:   5000, // Different!
				Status:        "Completed",
				TransactionID: "txn_abc123",
			},
			serverStatus: http.StatusOK,
			wantErr:      core.ErrAmountMismatch,
		},
		{
			name: "missing pidx",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{}, // No pidx
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "payment not found",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{"pidx": "invalid_pidx"},
			},
			serverStatus: http.StatusNotFound,
			wantErr:      core.ErrPaymentNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server

			if tt.serverResponse != nil || tt.serverStatus != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, lookupEndpoint, r.URL.Path)

					if tt.serverStatus != 0 && tt.serverStatus != http.StatusOK {
						w.WriteHeader(tt.serverStatus)
						return
					}

					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.serverResponse) //nolint:errcheck
				}))
				defer server.Close()
			}

			cfg := Config{
				SecretKey:  "test_secret",
				WebsiteURL: "https://example.com",
			}
			if server != nil {
				cfg.BaseURL = server.URL
			}

			provider, err := New(cfg)
			require.NoError(t, err)

			resp, err := provider.VerifyPayment(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSuccess, resp.Success)
				assert.Equal(t, tt.wantStatus, resp.Status)
				assert.Equal(t, tt.wantAmount, resp.VerifiedAmount)
				assert.Equal(t, tt.wantTxnID, resp.TransactionID)
				assert.Equal(t, "NPR", resp.Currency)
			}
		})
	}
}

func TestStatusNormalization(t *testing.T) {
	tests := []struct {
		khaltiStatus   string
		expectedStatus string
		expectedOK     bool
	}{
		{"Completed", core.StatusComplete, true},
		{"Pending", core.StatusPending, false},
		{"Initiated", core.StatusPending, false},
		{"Refunded", core.StatusRefunded, false},
		{"Partially refunded", core.StatusRefunded, false},
		{"Expired", core.StatusExpired, false},
		{"User canceled", core.StatusCancelled, false},
		{"Unknown", core.StatusFailed, false},
		{"SomeNewStatus", core.StatusFailed, false},
	}

	for _, tt := range tests {
		t.Run(tt.khaltiStatus, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(lookupResponse{ //nolint:errcheck
					PIDX:        "test",
					TotalAmount: 1000,
					Status:      tt.khaltiStatus,
				})
			}))
			defer server.Close()

			provider, _ := New(Config{
				SecretKey:  "test_secret",
				WebsiteURL: "https://example.com",
				BaseURL:    server.URL,
			})

			resp, err := provider.VerifyPayment(context.Background(), &core.VerifyRequest{
				EncodedParams: map[string]string{"pidx": "test"},
			})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.Status)
			assert.Equal(t, tt.expectedOK, resp.Success)
		})
	}
}

func TestHTTPErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
		wantText   string
	}{
		{"rate limited", http.StatusTooManyRequests, "", core.ErrRateLimited, ""},
		{"unauthorized", http.StatusUnauthorized, "", core.ErrInvalidConfig, ""},
		{"unauthorized with detail", http.StatusUnauthorized, `{"detail":"Invalid token.","status_code":401}`, core.ErrInvalidConfig, "Invalid token."},
		{"not found", http.StatusNotFound, "", core.ErrPaymentNotFound, ""},
		{"bad request", http.StatusBadRequest, "", core.ErrInvalidRequest, ""},
		{"validation error", http.StatusBadRequest, `{"purchase_order_name":["This field may not be blank."],"error_key":"validation_error"}`, core.ErrInvalidRequest, "purchase_order_name"},
		{"server error", http.StatusInternalServerError, "", core.ErrProviderError, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider, _ := New(Config{
				SecretKey:  "test_secret",
				WebsiteURL: "https://example.com",
				BaseURL:    server.URL,
			})

			_, err := provider.InitiatePayment(context.Background(), &core.PaymentRequest{
				Amount:      1000,
				OrderID:     "X",
				CallbackURL: "http://x",
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), tt.wantText)
		})
	}
}

// Khalti's lookup answers cancelled and expired payments with HTTP 400 and the normal body.
func TestVerifyPaymentLookup400WithStatus(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus string
	}{
		{"user canceled", `{"pidx":"vNTeXkSEaEXK2J4i7cQU6e","total_amount":1000,"status":"User canceled","transaction_id":null,"fee":0,"refunded":false}`, core.StatusCancelled},
		{"expired", `{"pidx":"vNTeXkSEaEXK2J4i7cQU6e","total_amount":1000,"status":"Expired","transaction_id":null,"fee":0,"refunded":false}`, core.StatusExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider, err := New(Config{SecretKey: "test_secret", WebsiteURL: "https://example.com", BaseURL: server.URL})
			require.NoError(t, err)

			resp, err := provider.VerifyPayment(context.Background(), &core.VerifyRequest{
				EncodedParams:  map[string]string{"pidx": "vNTeXkSEaEXK2J4i7cQU6e"},
				ExpectedAmount: 1000,
			})
			require.NoError(t, err)
			assert.False(t, resp.Success)
			assert.Equal(t, tt.wantStatus, resp.Status)
		})
	}
}

func TestCheckStatusAndProviderRef(t *testing.T) {
	tests := []struct {
		name        string
		call        func(p *Provider) (*core.VerifyResponse, error)
		reply       *lookupResponse
		wantErr     error
		wantStatus  string
		wantSuccess bool
	}{
		{
			name: "check status paid",
			call: func(p *Provider) (*core.VerifyResponse, error) {
				return p.CheckStatus(context.Background(), &core.StatusRequest{OrderID: "ORDER-1", ProviderRef: "test_pidx", Amount: 10000})
			},
			reply:       &lookupResponse{PIDX: "test_pidx", TotalAmount: 10000, Status: "Completed", TransactionID: "txn_1"},
			wantStatus:  core.StatusComplete,
			wantSuccess: true,
		},
		{
			name: "check status pending",
			call: func(p *Provider) (*core.VerifyResponse, error) {
				return p.CheckStatus(context.Background(), &core.StatusRequest{ProviderRef: "test_pidx"})
			},
			reply:      &lookupResponse{PIDX: "test_pidx", TotalAmount: 10000, Status: "Pending"},
			wantStatus: core.StatusPending,
		},
		{
			name: "check status amount mismatch",
			call: func(p *Provider) (*core.VerifyResponse, error) {
				return p.CheckStatus(context.Background(), &core.StatusRequest{ProviderRef: "test_pidx", Amount: 500})
			},
			reply:   &lookupResponse{PIDX: "test_pidx", TotalAmount: 10000, Status: "Completed"},
			wantErr: core.ErrAmountMismatch,
		},
		{
			name: "check status missing pidx",
			call: func(p *Provider) (*core.VerifyResponse, error) {
				return p.CheckStatus(context.Background(), &core.StatusRequest{OrderID: "ORDER-1", Amount: 10000})
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "verify with matching provider ref",
			call: func(p *Provider) (*core.VerifyResponse, error) {
				return p.VerifyPayment(context.Background(), &core.VerifyRequest{EncodedParams: map[string]string{"pidx": "test_pidx"}, ProviderRef: "test_pidx"})
			},
			reply:       &lookupResponse{PIDX: "test_pidx", TotalAmount: 10000, Status: "Completed"},
			wantStatus:  core.StatusComplete,
			wantSuccess: true,
		},
		{
			name: "verify with other provider ref",
			call: func(p *Provider) (*core.VerifyResponse, error) {
				return p.VerifyPayment(context.Background(), &core.VerifyRequest{EncodedParams: map[string]string{"pidx": "other_pidx"}, ProviderRef: "test_pidx"})
			},
			wantErr: core.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.reply == nil {
					t.Error("unexpected lookup call")
				}
				var body lookupRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "test_pidx", body.PIDX)
				_ = json.NewEncoder(w).Encode(tt.reply) //nolint:errcheck
			}))
			defer server.Close()

			provider, err := New(Config{SecretKey: "test_secret", WebsiteURL: "https://example.com", BaseURL: server.URL})
			require.NoError(t, err)

			resp, err := tt.call(provider)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.Status)
			assert.Equal(t, tt.wantSuccess, resp.Success)
		})
	}
}
