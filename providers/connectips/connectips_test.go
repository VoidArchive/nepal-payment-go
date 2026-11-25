package connectips

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/v2/core"
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
			name: "pfx path rejected",
			config: Config{
				MerchantID: "MERCHANT001",
				AppID:      "APP001",
				CertPath:   "/path/to/CREDITOR.PFX",
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

// verifyToken checks a base64 SHA256withRSA token against message with the public key.
func verifyToken(t *testing.T, key *rsa.PrivateKey, message, token string) {
	t.Helper()
	sig, err := base64.StdEncoding.DecodeString(token)
	require.NoError(t, err)
	hash := sha256.Sum256([]byte(message))
	assert.NoError(t, rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], sig), "token does not sign %q", message)
}

func TestInitiationTokenFormat(t *testing.T) {
	key, pemData := generateTestKey(t)
	provider, err := New(Config{
		MerchantID: "550",
		AppID:      "MER-550-APP-1",
		AppName:    "Test",
		CertData:   pemData,
		Sandbox:    true,
	})
	require.NoError(t, err)

	resp, err := provider.InitiatePayment(context.Background(), &core.PaymentRequest{
		Amount:      500,
		OrderID:     "txn123",
		Description: "PART-001",
		CallbackURL: "https://example.com/callback",
	})
	require.NoError(t, err)

	f := resp.FormFields
	message := "MERCHANTID=550,APPID=MER-550-APP-1,APPNAME=Test,TXNID=txn123,TXNDATE=" + f["TXNDATE"] +
		",TXNCRNCY=NPR,TXNAMT=500,REFERENCEID=txn123,REMARKS=PART-001,PARTICULARS=PART-001,TOKEN=TOKEN"
	verifyToken(t, key, message, f["TOKEN"])
}

func TestVerifyPayment(t *testing.T) {
	key, pemData := generateTestKey(t)

	tests := []struct {
		name         string
		request      *core.VerifyRequest
		serverBody   string
		serverStatus int
		wantErr      error
		wantSuccess  bool
		wantStatus   string
		wantAmount   int64
	}{
		{
			name: "successful payment, string amount",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "ORDER-123"},
				ExpectedAmount: 500,
			},
			serverBody:   `{"merchantId":550,"appId":"MER-550-APP-1","referenceId":"ORDER-123","txnAmt":"500","token":null,"status":"SUCCESS","statusDesc":"TRANSACTION SUCCESSFUL"}`,
			serverStatus: http.StatusOK,
			wantSuccess:  true,
			wantStatus:   core.StatusComplete,
			wantAmount:   500,
		},
		{
			name: "successful payment, numeric amount",
			request: &core.VerifyRequest{
				EncodedParams:   map[string]string{"TXNID": "ORDER-123"},
				OriginalOrderID: "ORDER-123",
				ExpectedAmount:  1000,
			},
			serverBody:   `{"merchantId":550,"appId":"MER-550-APP-1","referenceId":"ORDER-123","txnAmt":1000.00,"token":null,"status":"SUCCESS","statusDesc":"TRANSACTION SUCCESSFUL"}`,
			serverStatus: http.StatusOK,
			wantSuccess:  true,
			wantStatus:   core.StatusComplete,
			wantAmount:   1000,
		},
		{
			name: "failed payment",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "ORDER-123"},
				ExpectedAmount: 1000,
			},
			serverBody:   `{"merchantId":550,"appId":"MER-550-APP-1","referenceId":"ORDER-123","txnAmt":"1000","token":null,"status":"FAILED","statusDesc":"TRANSACTION FAILED"}`,
			serverStatus: http.StatusOK,
			wantSuccess:  false,
			wantStatus:   core.StatusFailed,
			wantAmount:   1000,
		},
		{
			name: "amount mismatch",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "ORDER-123"},
				ExpectedAmount: 1000,
			},
			serverBody:   `{"merchantId":550,"appId":"MER-550-APP-1","referenceId":"ORDER-123","txnAmt":"2000","token":null,"status":"SUCCESS","statusDesc":"TRANSACTION SUCCESSFUL"}`,
			serverStatus: http.StatusOK,
			wantErr:      core.ErrAmountMismatch,
		},
		{
			name: "reply referenceId differs",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "ORDER-123"},
				ExpectedAmount: 1000,
			},
			serverBody:   `{"merchantId":550,"appId":"MER-550-APP-1","referenceId":"ORDER-999","txnAmt":"1000","token":null,"status":"SUCCESS","statusDesc":"TRANSACTION SUCCESSFUL"}`,
			serverStatus: http.StatusOK,
			wantErr:      core.ErrInvalidRequest,
		},
		{
			name: "transaction not found",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "INVALID-ORDER"},
				ExpectedAmount: 1000,
			},
			serverBody:   `{"status":"NOT_FOUND"}`,
			serverStatus: http.StatusOK,
			wantErr:      core.ErrPaymentNotFound,
		},
		{
			name: "invalid token",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "ORDER-123"},
				ExpectedAmount: 1000,
			},
			serverBody:   `{"status":"INVALID_TOKEN"}`,
			serverStatus: http.StatusOK,
			wantErr:      core.ErrSignatureMismatch,
		},
		{
			name: "server error",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{"TXNID": "ORDER-123"},
				ExpectedAmount: 1000,
			},
			serverStatus: http.StatusInternalServerError,
			wantErr:      core.ErrProviderError,
		},
		{
			name: "missing TXNID",
			request: &core.VerifyRequest{
				EncodedParams:  map[string]string{},
				ExpectedAmount: 1000,
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "missing ExpectedAmount",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{"TXNID": "ORDER-123"},
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "callback TXNID differs from OriginalOrderID",
			request: &core.VerifyRequest{
				EncodedParams:   map[string]string{"TXNID": "ORDER-999"},
				OriginalOrderID: "ORDER-123",
				ExpectedAmount:  1000,
			},
			wantErr: core.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Contains(t, r.URL.Path, "/api/creditor/validatetxn")
				user, pass, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "APP001", user)
				assert.Equal(t, "secret", pass)

				var body validationRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				verifyToken(t, key, fmt.Sprintf("MERCHANTID=MERCHANT001,APPID=APP001,REFERENCEID=%s,TXNAMT=%d",
					body.ReferenceID, body.TxnAmount), body.Token)

				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			provider, err := New(Config{
				MerchantID:  "MERCHANT001",
				AppID:       "APP001",
				AppPassword: "secret",
				CertData:    pemData,
				Sandbox:     true,
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
				assert.Equal(t, tt.wantAmount, resp.VerifiedAmount)
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

func TestCheckStatus(t *testing.T) {
	_, pemData := generateTestKey(t)

	tests := []struct {
		name        string
		request     *core.StatusRequest
		serverBody  string
		wantErr     error
		wantStatus  string
		wantSuccess bool
	}{
		{
			name:        "paid",
			request:     &core.StatusRequest{OrderID: "ORDER-123", Amount: 500},
			serverBody:  `{"referenceId":"ORDER-123","txnAmt":"500","status":"SUCCESS"}`,
			wantStatus:  core.StatusComplete,
			wantSuccess: true,
		},
		{
			name:       "pending",
			request:    &core.StatusRequest{OrderID: "ORDER-123", Amount: 500},
			serverBody: `{"referenceId":"ORDER-123","txnAmt":"500","status":"PENDING"}`,
			wantStatus: core.StatusPending,
		},
		{
			name:       "amount mismatch",
			request:    &core.StatusRequest{OrderID: "ORDER-123", Amount: 500},
			serverBody: `{"referenceId":"ORDER-123","txnAmt":"900","status":"SUCCESS"}`,
			wantErr:    core.ErrAmountMismatch,
		},
		{name: "missing order ID", request: &core.StatusRequest{Amount: 500}, wantErr: core.ErrInvalidRequest},
		{name: "missing amount", request: &core.StatusRequest{OrderID: "ORDER-123"}, wantErr: core.ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body validationRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "ORDER-123", body.ReferenceID)
				assert.Equal(t, int64(500), body.TxnAmount)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			provider, err := New(Config{MerchantID: "MERCHANT001", AppID: "APP001", AppPassword: "secret", CertData: pemData, Sandbox: true})
			require.NoError(t, err)
			provider.validationURL = server.URL

			resp, err := provider.CheckStatus(context.Background(), tt.request)
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
