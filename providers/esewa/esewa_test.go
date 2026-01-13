package esewa

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/core"
)

const testSecretKey = "8gBm/:&EnhH.1/q"

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
				SecretKey:   testSecretKey,
				ProductCode: "EPAYTEST",
				Sandbox:     true,
			},
			wantErr: nil,
			wantNil: false,
		},
		{
			name: "missing secret key",
			config: Config{
				ProductCode: "EPAYTEST",
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "missing product code",
			config: Config{
				SecretKey: testSecretKey,
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "production mode",
			config: Config{
				SecretKey:   testSecretKey,
				ProductCode: "PROD123",
				Sandbox:     false,
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
				Amount:      100,
				Currency:    "NPR",
				OrderID:     "ORDER-123",
				Description: "Test Payment",
				CallbackURL: "https://example.com/callback",
			},
			wantErr:      nil,
			wantFormPOST: true,
			wantOrderID:  "ORDER-123",
			checkFormKeys: []string{
				"amount", "total_amount", "transaction_uuid",
				"product_code", "signature", "signed_field_names",
				"success_url", "failure_url",
			},
		},
		{
			name: "with success/failure URLs from config",
			request: &core.PaymentRequest{
				Amount:      200,
				OrderID:     "ORDER-456",
				CallbackURL: "https://fallback.com/callback",
			},
			wantErr:      nil,
			wantFormPOST: true,
			wantOrderID:  "ORDER-456",
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
				SecretKey:   testSecretKey,
				ProductCode: "EPAYTEST",
				Sandbox:     true,
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
				assert.Equal(t, tt.wantFormPOST, resp.IsFormPOST, "eSewa should use form POST")
				assert.Equal(t, tt.wantOrderID, resp.TransactionID)
				assert.Equal(t, sandboxFormURL, resp.TargetURL)
				assert.NotEmpty(t, resp.FormFields)

				for _, key := range tt.checkFormKeys {
					assert.Contains(t, resp.FormFields, key, "missing form field: %s", key)
					assert.NotEmpty(t, resp.FormFields[key], "empty form field: %s", key)
				}
			}
		})
	}
}

func TestSignatureGeneration(t *testing.T) {
	provider, err := New(Config{
		SecretKey:   testSecretKey,
		ProductCode: "EPAYTEST",
		Sandbox:     true,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "standard payment signature",
			message: "total_amount=100,transaction_uuid=ORDER-123,product_code=EPAYTEST",
		},
		{
			name:    "larger amount",
			message: "total_amount=50000,transaction_uuid=ORDER-789,product_code=EPAYTEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := provider.generateSignature(tt.message)

			// Verify signature format (Base64)
			assert.NotEmpty(t, sig)
			_, err := base64.StdEncoding.DecodeString(sig)
			assert.NoError(t, err, "signature should be valid Base64")

			// Verify signature is reproducible
			sig2 := provider.generateSignature(tt.message)
			assert.Equal(t, sig, sig2, "same message should produce same signature")

			// Verify different message produces different signature
			sig3 := provider.generateSignature(tt.message + "x")
			assert.NotEqual(t, sig, sig3, "different message should produce different signature")
		})
	}
}

func TestVerifyPayment(t *testing.T) {
	// Helper to create valid Base64-encoded callback
	createCallback := func(secretKey string, status string, amount float64, uuid string) string {
		amountStr := fmt.Sprintf("%.1f", amount)
		callback := callbackResponse{
			TransactionCode:  "000AWEO",
			Status:           status,
			TotalAmount:      amountStr, // eSewa returns as string
			TransactionUUID:  uuid,
			ProductCode:      "EPAYTEST",
			SignedFieldNames: "transaction_code,status,total_amount,transaction_uuid,product_code,signed_field_names",
		}

		// Generate signature using string value
		message := fmt.Sprintf("transaction_code=%s,status=%s,total_amount=%s,transaction_uuid=%s,product_code=%s,signed_field_names=%s",
			callback.TransactionCode, callback.Status, callback.TotalAmount,
			callback.TransactionUUID, callback.ProductCode, callback.SignedFieldNames)

		mac := hmac.New(sha256.New, []byte(secretKey))
		mac.Write([]byte(message))
		callback.Signature = base64.StdEncoding.EncodeToString(mac.Sum(nil))

		data, _ := json.Marshal(callback)
		return base64.StdEncoding.EncodeToString(data)
	}

	tests := []struct {
		name        string
		request     *core.VerifyRequest
		wantErr     error
		wantSuccess bool
		wantStatus  string
		wantAmount  int64
		wantTxnID   string
	}{
		{
			name: "completed payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"data": createCallback(testSecretKey, "COMPLETE", 100.0, "ORDER-123"),
				},
				ExpectedAmount: 100,
			},
			wantErr:     nil,
			wantSuccess: true,
			wantStatus:  core.StatusComplete,
			wantAmount:  100,
			wantTxnID:   "000AWEO",
		},
		{
			name: "pending payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"data": createCallback(testSecretKey, "PENDING", 500.0, "ORDER-456"),
				},
			},
			wantErr:     nil,
			wantSuccess: false,
			wantStatus:  core.StatusPending,
			wantAmount:  500,
		},
		{
			name: "amount mismatch",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"data": createCallback(testSecretKey, "COMPLETE", 200.0, "ORDER-123"),
				},
				ExpectedAmount: 100, // Different!
			},
			wantErr: core.ErrAmountMismatch,
		},
		{
			name: "invalid signature",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"data": createCallback("wrong-secret", "COMPLETE", 100.0, "ORDER-123"),
				},
			},
			wantErr: core.ErrSignatureMismatch,
		},
		{
			name: "invalid base64",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"data": "not-valid-base64!!!",
				},
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "invalid json",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"data": base64.StdEncoding.EncodeToString([]byte("not json")),
				},
			},
			wantErr: core.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := New(Config{
				SecretKey:   testSecretKey,
				ProductCode: "EPAYTEST",
				Sandbox:     true,
			})
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
				if tt.wantTxnID != "" {
					assert.Equal(t, tt.wantTxnID, resp.TransactionID)
				}
				assert.Equal(t, "NPR", resp.Currency)
			}
		})
	}
}

func TestVerifyViaStatusAPI(t *testing.T) {
	tests := []struct {
		name           string
		request        *core.VerifyRequest
		serverResponse *statusResponse
		serverStatus   int
		wantErr        error
		wantSuccess    bool
		wantStatus     string
	}{
		{
			name: "completed via status API",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"transaction_uuid": "ORDER-123",
				},
				ExpectedAmount: 100,
			},
			serverResponse: &statusResponse{
				ProductCode:     "EPAYTEST",
				TransactionUUID: "ORDER-123",
				TotalAmount:     100.0,
				Status:          "COMPLETE",
				RefID:           strPtr("0001TS9"),
			},
			serverStatus: http.StatusOK,
			wantErr:      nil,
			wantSuccess:  true,
			wantStatus:   core.StatusComplete,
		},
		{
			name: "not found",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"transaction_uuid": "INVALID-ORDER",
				},
				ExpectedAmount: 100,
			},
			serverResponse: &statusResponse{
				Status: "NOT_FOUND",
			},
			serverStatus: http.StatusOK,
			wantErr:      core.ErrPaymentNotFound,
		},
		{
			name: "amount mismatch via API",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"transaction_uuid": "ORDER-123",
				},
				ExpectedAmount: 100,
			},
			serverResponse: &statusResponse{
				ProductCode:     "EPAYTEST",
				TransactionUUID: "ORDER-123",
				TotalAmount:     200.0, // Different!
				Status:          "COMPLETE",
			},
			serverStatus: http.StatusOK,
			wantErr:      core.ErrAmountMismatch,
		},
		{
			name: "server error",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"transaction_uuid": "ORDER-123",
				},
				ExpectedAmount: 100,
			},
			serverStatus: http.StatusInternalServerError,
			wantErr:      core.ErrProviderError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Contains(t, r.URL.Path, "/api/epay/transaction/status")

				if tt.serverStatus != http.StatusOK {
					w.WriteHeader(tt.serverStatus)
					return
				}

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			provider, err := New(Config{
				SecretKey:   testSecretKey,
				ProductCode: "EPAYTEST",
				Sandbox:     true,
			})
			require.NoError(t, err)
			provider.statusURL = server.URL + "/api/epay/transaction/status/"

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
	tests := []struct {
		esewaStatus    string
		expectedStatus string
		expectedOK     bool
	}{
		{"COMPLETE", core.StatusComplete, true},
		{"PENDING", core.StatusPending, false},
		{"FULL_REFUND", core.StatusRefunded, false},
		{"PARTIAL_REFUND", core.StatusRefunded, false},
		{"CANCELED", core.StatusCancelled, false},
		{"AMBIGUOUS", core.StatusFailed, false},
		{"NOT_FOUND", core.StatusFailed, false},
		{"UNKNOWN", core.StatusFailed, false},
	}

	provider, _ := New(Config{
		SecretKey:   testSecretKey,
		ProductCode: "EPAYTEST",
		Sandbox:     true,
	})

	for _, tt := range tests {
		t.Run(tt.esewaStatus, func(t *testing.T) {
			status, ok := provider.normalizeStatus(tt.esewaStatus)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedOK, ok)
		})
	}
}

func strPtr(s string) *string {
	return &s
}
