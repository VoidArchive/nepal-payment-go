package fonepay

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voidarchive/nepal-payment-go/core"
)

const testSecretKey = "a7e3512f5032480a83137793cb2021dc"

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
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
			},
			wantErr: nil,
			wantNil: false,
		},
		{
			name: "missing merchant code",
			config: Config{
				SecretKey: testSecretKey,
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "missing secret key",
			config: Config{
				MerchantCode: "NBQM",
			},
			wantErr: core.ErrInvalidConfig,
			wantNil: true,
		},
		{
			name: "production mode",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      false,
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
		name             string
		request          *core.PaymentRequest
		wantErr          error
		wantFormPOST     bool
		wantOrderID      string
		checkURLContains []string
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
			wantFormPOST: false, // FonePay uses redirect
			wantOrderID:  "ORDER-123",
			checkURLContains: []string{
				"PID=NBQM",
				"PRN=ORDER-123",
				"AMT=1000",
				"CRN=NPR",
				"DV=",
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
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
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
				assert.Equal(t, tt.wantFormPOST, resp.IsFormPOST, "FonePay should use redirect")
				assert.Equal(t, tt.wantOrderID, resp.TransactionID)
				assert.Contains(t, resp.TargetURL, sandboxBaseURL)

				for _, substr := range tt.checkURLContains {
					assert.Contains(t, resp.TargetURL, substr, "URL should contain: %s", substr)
				}
			}
		})
	}
}

func TestHMACGeneration(t *testing.T) {
	provider, err := New(Config{
		MerchantCode: "NBQM",
		SecretKey:    testSecretKey,
		Sandbox:      true,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "standard payment",
			message: "NBQM,P,ORDER-123,1000,NPR,01/14/2026,Test,",
		},
		{
			name:    "larger amount",
			message: "NBQM,P,ORDER-789,500000,NPR,01/15/2026,Large Payment,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := provider.generateHMAC(tt.message)

			// Hash should be non-empty hex string
			assert.NotEmpty(t, hash)
			_, err := hex.DecodeString(hash)
			assert.NoError(t, err, "hash should be valid hex")

			// Hash should be reproducible
			hash2 := provider.generateHMAC(tt.message)
			assert.Equal(t, hash, hash2, "same message should produce same hash")

			// Different message should produce different hash
			hash3 := provider.generateHMAC(tt.message + "x")
			assert.NotEqual(t, hash, hash3, "different message should produce different hash")
		})
	}
}

func TestVerifyPayment(t *testing.T) {
	// Helper to generate valid HMAC for callback
	generateCallbackDV := func(prn, bid, uid string) string {
		message := fmt.Sprintf("%s,%s,%s", prn, bid, uid)
		mac := hmac.New(sha512.New, []byte(testSecretKey))
		mac.Write([]byte(message))
		return hex.EncodeToString(mac.Sum(nil))
	}

	tests := []struct {
		name        string
		request     *core.VerifyRequest
		wantErr     error
		wantSuccess bool
		wantStatus  string
	}{
		{
			name: "successful payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"PRN": "ORDER-123",
					"BID": "BANK001",
					"UID": "TXN123456",
					"P":   "true",
					"PS":  "success",
					"RC":  "successful",
					"DV":  generateCallbackDV("ORDER-123", "BANK001", "TXN123456"),
				},
			},
			wantErr:     nil,
			wantSuccess: true,
			wantStatus:  core.StatusComplete,
		},
		{
			name: "failed payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"PRN": "ORDER-123",
					"BID": "BANK001",
					"UID": "TXN123456",
					"P":   "false",
					"PS":  "failed",
					"RC":  "failed",
					"DV":  generateCallbackDV("ORDER-123", "BANK001", "TXN123456"),
				},
			},
			wantErr:     nil,
			wantSuccess: false,
			wantStatus:  core.StatusFailed,
		},
		{
			name: "cancelled payment",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"PRN": "ORDER-123",
					"BID": "BANK001",
					"UID": "TXN123456",
					"P":   "false",
					"RC":  "cancelled",
					"DV":  generateCallbackDV("ORDER-123", "BANK001", "TXN123456"),
				},
			},
			wantErr:     nil,
			wantSuccess: false,
			wantStatus:  core.StatusCancelled,
		},
		{
			name: "invalid signature",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"PRN": "ORDER-123",
					"BID": "BANK001",
					"UID": "TXN123456",
					"P":   "true",
					"DV":  "invalid_signature_here",
				},
			},
			wantErr: core.ErrSignatureMismatch,
		},
		{
			name: "missing PRN",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{},
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "amount mismatch",
			request: &core.VerifyRequest{
				EncodedParams: map[string]string{
					"PRN": "ORDER-123",
					"BID": "BANK001",
					"UID": "TXN123456",
					"P":   "true",
					"AMT": "2000",
					"DV":  generateCallbackDV("ORDER-123", "BANK001", "TXN123456"),
				},
				ExpectedAmount: 1000,
			},
			wantErr: core.ErrAmountMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := New(Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
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
			}
		})
	}
}

func TestStatusNormalization(t *testing.T) {
	tests := []struct {
		status         string
		rc             string
		expectedStatus string
		expectedOK     bool
	}{
		{"true", "", core.StatusComplete, true},
		{"True", "", core.StatusComplete, true},
		{"TRUE", "", core.StatusComplete, true},
		{"false", "successful", core.StatusComplete, true},
		{"false", "pending", core.StatusPending, false},
		{"false", "cancelled", core.StatusCancelled, false},
		{"false", "canceled", core.StatusCancelled, false},
		{"false", "failed", core.StatusFailed, false},
		{"false", "", core.StatusFailed, false},
	}

	provider, _ := New(Config{
		MerchantCode: "NBQM",
		SecretKey:    testSecretKey,
		Sandbox:      true,
	})

	for _, tt := range tests {
		name := fmt.Sprintf("P=%s,RC=%s", tt.status, tt.rc)
		t.Run(name, func(t *testing.T) {
			status, ok := provider.normalizeStatus(tt.status, tt.rc)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedOK, ok)
		})
	}
}

func TestURLGeneration(t *testing.T) {
	provider, err := New(Config{
		MerchantCode: "NBQM",
		SecretKey:    testSecretKey,
		Sandbox:      true,
	})
	require.NoError(t, err)

	resp, err := provider.InitiatePayment(context.Background(), &core.PaymentRequest{
		Amount:      100,
		OrderID:     "TEST-001",
		Description: "Test",
		CallbackURL: "http://localhost:8080/callback",
	})
	require.NoError(t, err)

	// URL should be properly formed
	assert.True(t, strings.HasPrefix(resp.TargetURL, sandboxBaseURL))
	assert.Contains(t, resp.TargetURL, "PID=NBQM")
	assert.Contains(t, resp.TargetURL, "PRN=TEST-001")
	assert.Contains(t, resp.TargetURL, "AMT=100")
	assert.Contains(t, resp.TargetURL, "MD=P")
	assert.Contains(t, resp.TargetURL, "CRN=NPR")
}

func TestGenerateQR_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		request *QRRequest
		wantErr error
	}{
		{
			name: "missing username",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
				Password:     "testpass",
			},
			request: &QRRequest{
				Amount:      100,
				OrderID:     "QR-001",
				Description: "Test QR",
			},
			wantErr: core.ErrInvalidConfig,
		},
		{
			name: "missing password",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
				Username:     "testuser",
			},
			request: &QRRequest{
				Amount:      100,
				OrderID:     "QR-001",
				Description: "Test QR",
			},
			wantErr: core.ErrInvalidConfig,
		},
		{
			name: "zero amount",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
				Username:     "testuser",
				Password:     "testpass",
			},
			request: &QRRequest{
				Amount:      0,
				OrderID:     "QR-001",
				Description: "Test QR",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "negative amount",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
				Username:     "testuser",
				Password:     "testpass",
			},
			request: &QRRequest{
				Amount:      -100,
				OrderID:     "QR-001",
				Description: "Test QR",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "missing order ID",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
				Username:     "testuser",
				Password:     "testpass",
			},
			request: &QRRequest{
				Amount:      100,
				Description: "Test QR",
			},
			wantErr: core.ErrInvalidRequest,
		},
		{
			name: "missing description",
			config: Config{
				MerchantCode: "NBQM",
				SecretKey:    testSecretKey,
				Sandbox:      true,
				Username:     "testuser",
				Password:     "testpass",
			},
			request: &QRRequest{
				Amount:  100,
				OrderID: "QR-001",
			},
			wantErr: core.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := New(tt.config)
			require.NoError(t, err)

			resp, err := provider.GenerateQR(context.Background(), tt.request)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, resp)
		})
	}
}

func TestQRAPIURL(t *testing.T) {
	// Sandbox should use dev URL
	sandboxProvider, _ := New(Config{
		MerchantCode: "NBQM",
		SecretKey:    testSecretKey,
		Sandbox:      true,
	})
	assert.Equal(t, sandboxQRURL, sandboxProvider.qrAPIURL)

	// Production should use production URL
	prodProvider, _ := New(Config{
		MerchantCode: "NBQM",
		SecretKey:    testSecretKey,
		Sandbox:      false,
	})
	assert.Equal(t, productionQRURL, prodProvider.qrAPIURL)
}
