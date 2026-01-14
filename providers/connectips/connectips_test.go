package connectips

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/core"
)

// generateTestKey creates an RSA key pair for testing.
func generateTestKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode to PEM
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}
	pemData := pem.EncodeToMemory(pemBlock)
	return key, pemData
}

func TestNew(t *testing.T) {
	_, pemData := generateTestKey(t)

	tests := []struct {
		name    string
		config  Config
		wantErr error
		wantNil bool
	}{
		{
			name: "valid config",
			config: Config{
				MerchantID: "MERCHANT001",
				AppID:      "APP001",
				AppName:    "Test App",
				CertData:   pemData,
				Sandbox:    true,
			},
			wantErr: nil,
			wantNil: false,
		},
		{
			name: "missing merchant ID",
			config: Config{
				AppID:    "APP001",
				CertData: pemData,
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "missing app ID",
			config: Config{
				MerchantID: "MERCHANT001",
				CertData:   pemData,
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "missing certificate",
			config: Config{
				MerchantID: "MERCHANT001",
				AppID:      "APP001",
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "production mode",
			config: Config{
				MerchantID: "MERCHANT001",
				AppID:      "APP001",
				CertData:   pemData,
				Sandbox:    false,
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
	_, pemData := generateTestKey(t)

	tests := []struct {
		name          string
		request       *core.PaymentRequest
		wantErr       error
		wantFormPOST  bool
		wantOrderID   string
		checkFormKeys []string
	}{
		{
			name: "successful initiation",
			request: &core.PaymentRequest{
				Amount:      1000,
				Currency:    "NPR",
				OrderID:     "ORDER-123",
				Description: "Test Payment",
				CallbackURL: "https://example.com/callback",
			},
			wantErr:      nil,
			wantFormPOST: true,
			wantOrderID:  "ORDER-123",
			checkFormKeys: []string{
				"MERCHANTID", "APPID", "APPNAME", "TXNID", "TXNDATE",
				"TXNCRNCY", "TXNAMT", "REFERENCEID", "TOKEN", "SUCCESSURL", "FAILUREURL",
			},
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
				Amount:      100,
				CallbackURL: "https://example.com/callback",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "missing callback URL",
			request: &core.PaymentRequest{
				Amount:  100,
				OrderID: "ORDER-123",
			},
			wantErr: core.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := New(Config{
				MerchantID: "MERCHANT001",
				AppID:      "APP001",
				AppName:    "Test App",
				CertData:   pemData,
				Sandbox:    true,
			})
			require.NoError(t, err)

			resp, err := provider.InitiatePayment(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantFormPOST, resp.IsFormPOST, "ConnectIPS should use form POST")
				assert.Equal(t, tt.wantOrderID, resp.TransactionID)
				assert.Equal(t, sandboxLoginURL, resp.TargetURL)
				assert.NotEmpty(t, resp.FormFields)

				for _, key := range tt.checkFormKeys {
					assert.Contains(t, resp.FormFields, key, "missing form field: %s", key)
					assert.NotEmpty(t, resp.FormFields[key], "empty form field: %s", key)
				}
			}
		})
	}
}

func TestTokenGeneration(t *testing.T) {
	_, pemData := generateTestKey(t)

	provider, err := New(Config{
		MerchantID: "MERCHANT001",
		AppID:      "APP001",
		AppName:    "Test App",
		CertData:   pemData,
		Sandbox:    true,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		txnID   string
		amount  int64
		txnDate string
	}{
		{
			name:    "standard payment",
			txnID:   "ORDER-123",
			amount:  1000,
			txnDate: "14-01-2026",
		},
		{
			name:    "larger amount",
			txnID:   "ORDER-789",
			amount:  500000,
			txnDate: "15-01-2026",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := provider.generateToken(tt.txnID, tt.amount, tt.txnDate)
			require.NoError(t, err)

			// Token should be non-empty Base64
			assert.NotEmpty(t, token)

			// Token should be reproducible
			token2, err := provider.generateToken(tt.txnID, tt.amount, tt.txnDate)
			require.NoError(t, err)
			assert.Equal(t, token, token2, "same input should produce same token")

			// Different input should produce different token
			token3, err := provider.generateToken(tt.txnID+"x", tt.amount, tt.txnDate)
			require.NoError(t, err)
			assert.NotEqual(t, token, token3, "different input should produce different token")
		})
	}
}

func TestVerifyPayment(t *testing.T) {
	_, pemData := generateTestKey(t)

	tests := []struct {
		name           string
		request        *core.VerifyRequest
		serverResponse *validationResponse
		serverStatus   int
		wantErr        error
		wantSuccess    bool
		wantStatus     string
	}{
		{
			name: "successful payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID":  "ORDER-123",
					"STATUS": "SUCCESS",
				},
				ExpectedAmount: 1000,
			},
			serverResponse: &validationResponse{
				Status:      "SUCCESS",
				TransRefID:  "CIPS123456",
				ReferenceID: "ORDER-123",
				TxnAmount:   1000,
				TokenValid:  true,
			},
			serverStatus: http.StatusOK,
			wantErr:      nil,
			wantSuccess:  true,
			wantStatus:   core.StatusComplete,
		},
		{
			name: "failed callback status",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID":  "ORDER-123",
					"STATUS": "FAILED",
				},
			},
			// No server call expected for FAILED callback
			wantErr:     nil,
			wantSuccess: false,
			wantStatus:  core.StatusFailed,
		},
		{
			name: "cancelled callback status",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID":  "ORDER-123",
					"STATUS": "CANCELLED",
				},
			},
			wantErr:     nil,
			wantSuccess: false,
			wantStatus:  core.StatusCancelled,
		},
		{
			name: "amount mismatch",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID": "ORDER-123",
				},
				ExpectedAmount: 1000,
			},
			serverResponse: &validationResponse{
				Status:      "SUCCESS",
				TransRefID:  "CIPS123456",
				ReferenceID: "ORDER-123",
				TxnAmount:   2000, // Different amount!
				TokenValid:  true,
			},
			serverStatus: http.StatusOK,
			wantErr:      core.ErrAmountMismatch,
		},
		{
			name: "transaction not found",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID": "INVALID-ORDER",
				},
				ExpectedAmount: 1000,
			},
			serverResponse: &validationResponse{
				Status: "NOT_FOUND",
			},
			serverStatus: http.StatusOK,
			wantErr:      core.ErrPaymentNotFound,
		},
		{
			name: "invalid token",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID": "ORDER-123",
				},
				ExpectedAmount: 1000,
			},
			serverResponse: &validationResponse{
				Status:     "INVALID_TOKEN",
				TokenValid: false,
			},
			serverStatus: http.StatusOK,
			wantErr:      core.ErrSignatureMismatch,
		},
		{
			name: "server error",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"TXNID": "ORDER-123",
				},
				ExpectedAmount: 1000,
			},
			serverStatus: http.StatusInternalServerError,
			wantErr:      core.ErrProviderError,
		},
		{
			name: "missing TXNID",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{},
			},
			wantErr: core.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip server setup for callback-only failures
			if tt.request.EncodedParams["STATUS"] == "FAILED" ||
				tt.request.EncodedParams["STATUS"] == "CANCELLED" {
				provider, err := New(Config{
					MerchantID: "MERCHANT001",
					AppID:      "APP001",
					CertData:   pemData,
					Sandbox:    true,
				})
				require.NoError(t, err)

				resp, err := provider.VerifyPayment(context.Background(), tt.request)
				require.NoError(t, err)
				assert.Equal(t, tt.wantSuccess, resp.Success)
				assert.Equal(t, tt.wantStatus, resp.Status)
				return
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Contains(t, r.URL.Path, "/api/creditor/validatetxn")

				if tt.serverStatus != http.StatusOK {
					w.WriteHeader(tt.serverStatus)
					return
				}

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(tt.serverResponse) //nolint:errcheck
			}))
			defer server.Close()

			provider, err := New(Config{
				MerchantID: "MERCHANT001",
				AppID:      "APP001",
				CertData:   pemData,
				Sandbox:    true,
			})
			require.NoError(t, err)
			provider.validationURL = server.URL + "/api/creditor/validatetxn"

			resp, err := provider.VerifyPayment(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantSuccess, resp.Success)
				assert.Equal(t, tt.wantStatus, resp.Status)
			}
		})
	}
}

func TestStatusNormalization(t *testing.T) {
	_, pemData := generateTestKey(t)

	tests := []struct {
		connectIPSStatus string
		expectedStatus   string
	}{
		{"SUCCESS", core.StatusComplete},
		{"PENDING", core.StatusPending},
		{"CANCELLED", core.StatusCancelled},
		{"TIMEOUT", core.StatusExpired},
		{"SESSION_EXPIRED", core.StatusExpired},
		{"FAILED", core.StatusFailed},
		{"NOT_FOUND", core.StatusFailed},
		{"INVALID_TOKEN", core.StatusFailed},
		{"UNKNOWN", core.StatusFailed},
	}

	provider, _ := New(Config{
		MerchantID: "MERCHANT001",
		AppID:      "APP001",
		CertData:   pemData,
		Sandbox:    true,
	})

	for _, tt := range tests {
		t.Run(tt.connectIPSStatus, func(t *testing.T) {
			status := provider.normalizeStatus(tt.connectIPSStatus)
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}
