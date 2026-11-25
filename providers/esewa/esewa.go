package esewa

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/voidarchive/nepal-payment-go/core"
)

const (
	sandboxFormURL   = "https://rc-epay.esewa.com.np/api/epay/main/v2/form"
	sandboxStatusURL = "https://rc.esewa.com.np/api/epay/transaction/status/"

	productionFormURL   = "https://epay.esewa.com.np/api/epay/main/v2/form"
	productionStatusURL = "https://esewa.com.np/api/epay/transaction/status/"
)

// Config holds eSewa provider configuration.
type Config struct {
	SecretKey   string        // Secret key from eSewa merchant dashboard
	ProductCode string        // Merchant product code (use "EPAYTEST" for sandbox)
	SuccessURL  string        // URL to redirect after successful payment
	FailureURL  string        // URL to redirect after failed payment
	Sandbox     bool          // Use sandbox environment
	Timeout     time.Duration // HTTP client timeout (default: 30s)
}

// Provider implements core.Provider for eSewa payments.
type Provider struct {
	config     Config
	httpClient *http.Client
	formURL    string
	statusURL  string
}

// New creates a new eSewa provider.
func New(cfg Config) (*Provider, error) {
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("%w: SecretKey is required", core.ErrInvalidConfig)
	}
	if cfg.ProductCode == "" {
		return nil, fmt.Errorf("%w: ProductCode is required", core.ErrInvalidConfig)
	}

	formURL := productionFormURL
	statusURL := productionStatusURL
	if cfg.Sandbox {
		formURL = sandboxFormURL
		statusURL = sandboxStatusURL
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Provider{
		config:     cfg,
		formURL:    formURL,
		statusURL:  statusURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// InitiatePayment creates form fields for eSewa payment.
// Returns IsFormPOST=true with FormFields to POST to TargetURL.
func (p *Provider) InitiatePayment(ctx context.Context, req *core.PaymentRequest) (*core.PaymentResponse, error) {
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// eSewa expects rupees; req.Amount is paisa. The signed string must match the sent field exactly.
	totalAmount := strconv.FormatFloat(float64(req.Amount)/100, 'f', -1, 64)

	// Generate signature for: total_amount,transaction_uuid,product_code
	signedFields := "total_amount,transaction_uuid,product_code"
	message := fmt.Sprintf("total_amount=%s,transaction_uuid=%s,product_code=%s",
		totalAmount, req.OrderID, p.config.ProductCode)
	signature := p.generateSignature(message)

	successURL := p.config.SuccessURL
	if successURL == "" {
		successURL = req.CallbackURL
	}
	failureURL := p.config.FailureURL
	if failureURL == "" {
		failureURL = req.CallbackURL
	}

	formFields := map[string]string{
		"amount":                  totalAmount,
		"tax_amount":              "0",
		"total_amount":            totalAmount,
		"transaction_uuid":        req.OrderID,
		"product_code":            p.config.ProductCode,
		"product_service_charge":  "0",
		"product_delivery_charge": "0",
		"success_url":             successURL,
		"failure_url":             failureURL,
		"signed_field_names":      signedFields,
		"signature":               signature,
	}

	return &core.PaymentResponse{
		IsFormPOST:    true,
		TargetURL:     p.formURL,
		FormFields:    formFields,
		TransactionID: req.OrderID,
	}, nil
}

// VerifyPayment validates callback data and confirms payment status.
// Expects EncodedParams["data"] to contain the Base64-encoded response.
func (p *Provider) VerifyPayment(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	// eSewa sends Base64 encoded data in the "data" query parameter
	encodedData := req.EncodedParams["data"]
	if encodedData == "" {
		// Fallback: try status check API if no callback data
		return p.verifyViaStatusAPI(ctx, req)
	}

	// Decode Base64 response
	decoded, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode callback data: %v", core.ErrInvalidRequest, err)
	}

	var callback callbackResponse
	if err := json.Unmarshal(decoded, &callback); err != nil {
		return nil, fmt.Errorf("%w: failed to parse callback JSON: %v", core.ErrInvalidRequest, err)
	}

	// Verify signature
	if err := p.verifyCallbackSignature(callback); err != nil {
		return nil, err
	}

	if req.OriginalOrderID != "" && req.OriginalOrderID != callback.TransactionUUID {
		return nil, fmt.Errorf("%w: order ID mismatch: expected %s, got %s",
			core.ErrInvalidRequest, req.OriginalOrderID, callback.TransactionUUID)
	}

	// eSewa returns rupees with thousands separators ("1,000.0"); convert to paisa.
	parsedAmount, err := strconv.ParseFloat(strings.ReplaceAll(callback.TotalAmount, ",", ""), 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid amount format: %v", core.ErrInvalidRequest, err)
	}
	verifiedAmount := int64(math.Round(parsedAmount * 100))

	// Validate amount if expected
	if req.ExpectedAmount > 0 && req.ExpectedAmount != verifiedAmount {
		return nil, fmt.Errorf("%w: expected %d, got %d",
			core.ErrAmountMismatch, req.ExpectedAmount, verifiedAmount)
	}

	status, success := p.normalizeStatus(callback.Status)

	return &core.VerifyResponse{
		Success:        success,
		TransactionID:  callback.TransactionCode,
		OrderID:        callback.TransactionUUID,
		VerifiedAmount: verifiedAmount,
		Currency:       "NPR",
		Status:         status,
		ProviderResponse: map[string]string{
			"transaction_code": callback.TransactionCode,
			"status":           callback.Status,
			"product_code":     callback.ProductCode,
		},
		VerifiedAt: time.Now().Unix(),
	}, nil
}

// verifyViaStatusAPI checks transaction status directly with eSewa API.
func (p *Provider) verifyViaStatusAPI(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	// The merchant's own order ID wins over the (attacker-controllable) URL param.
	transactionUUID := req.OriginalOrderID
	if transactionUUID == "" {
		transactionUUID = req.EncodedParams["transaction_uuid"]
	}
	if transactionUUID == "" {
		return nil, fmt.Errorf("%w: transaction_uuid or data parameter required", core.ErrInvalidRequest)
	}

	// eSewa expects rupees; ExpectedAmount is paisa. The URL param is already rupees as eSewa sent it.
	totalAmount := strconv.FormatFloat(float64(req.ExpectedAmount)/100, 'f', -1, 64)
	if req.ExpectedAmount == 0 {
		totalAmount = req.EncodedParams["total_amount"]
	}

	// Build status check URL
	statusURL := fmt.Sprintf("%s?product_code=%s&total_amount=%s&transaction_uuid=%s",
		p.statusURL,
		url.QueryEscape(p.config.ProductCode),
		url.QueryEscape(totalAmount),
		url.QueryEscape(transactionUUID))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %v", core.ErrProviderError, err)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", core.ErrTimeout, ctx.Err())
		}
		return nil, fmt.Errorf("%w: request failed: %v", core.ErrProviderError, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %v", core.ErrProviderError, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", core.ErrProviderError, resp.StatusCode)
	}

	var statusResp statusResponse
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse response: %v", core.ErrProviderError, err)
	}
	if statusResp.ErrorMessage != "" {
		return nil, fmt.Errorf("%w: %s", core.ErrProviderError, statusResp.ErrorMessage)
	}

	if statusResp.Status == "NOT_FOUND" {
		return nil, core.ErrPaymentNotFound
	}

	if req.OriginalOrderID != "" && req.OriginalOrderID != statusResp.TransactionUUID {
		return nil, fmt.Errorf("%w: order ID mismatch: expected %s, got %s",
			core.ErrInvalidRequest, req.OriginalOrderID, statusResp.TransactionUUID)
	}

	// Reply is rupees; convert to paisa.
	actualAmount := int64(math.Round(statusResp.TotalAmount * 100))
	if req.ExpectedAmount > 0 {
		if req.ExpectedAmount != actualAmount {
			return nil, fmt.Errorf("%w: expected %d, got %d",
				core.ErrAmountMismatch, req.ExpectedAmount, actualAmount)
		}
	}

	status, success := p.normalizeStatus(statusResp.Status)

	refID := ""
	if statusResp.RefID != nil {
		refID = *statusResp.RefID
	}

	return &core.VerifyResponse{
		Success:        success,
		TransactionID:  refID,
		OrderID:        statusResp.TransactionUUID,
		VerifiedAmount: actualAmount,
		Currency:       "NPR",
		Status:         status,
		ProviderResponse: map[string]string{
			"product_code":     statusResp.ProductCode,
			"status":           statusResp.Status,
			"ref_id":           refID,
			"transaction_uuid": statusResp.TransactionUUID,
		},
		VerifiedAt: time.Now().Unix(),
	}, nil
}

func (p *Provider) validateRequest(req *core.PaymentRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", core.ErrInvalidRequest)
	}
	if req.OrderID == "" {
		return fmt.Errorf("%w: OrderID is required", core.ErrInvalidRequest)
	}
	if req.CallbackURL == "" && p.config.SuccessURL == "" {
		return fmt.Errorf("%w: CallbackURL or SuccessURL is required", core.ErrInvalidRequest)
	}
	return nil
}

func (p *Provider) generateSignature(message string) string {
	mac := hmac.New(sha256.New, []byte(p.config.SecretKey))
	mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// callbackSignedFields is the only field set we accept: trusting the callback's own list lets
// the checkout-form signature (total_amount,transaction_uuid,product_code) verify a forged COMPLETE.
const callbackSignedFields = "transaction_code,status,total_amount,transaction_uuid,product_code,signed_field_names"

func (p *Provider) verifyCallbackSignature(callback callbackResponse) error {
	if callback.SignedFieldNames != callbackSignedFields {
		return fmt.Errorf("%w: unexpected signed_field_names", core.ErrSignatureMismatch)
	}
	if callback.ProductCode != p.config.ProductCode {
		return fmt.Errorf("%w: product code mismatch", core.ErrSignatureMismatch)
	}

	message := fmt.Sprintf("transaction_code=%s,status=%s,total_amount=%s,transaction_uuid=%s,product_code=%s,signed_field_names=%s",
		callback.TransactionCode, callback.Status, callback.TotalAmount,
		callback.TransactionUUID, callback.ProductCode, callback.SignedFieldNames)
	expectedSig := p.generateSignature(message)

	if !hmac.Equal([]byte(expectedSig), []byte(callback.Signature)) {
		return fmt.Errorf("%w: callback signature mismatch", core.ErrSignatureMismatch)
	}
	return nil
}

func (p *Provider) normalizeStatus(esewaStatus string) (string, bool) {
	switch esewaStatus {
	case "COMPLETE":
		return core.StatusComplete, true
	case "PENDING":
		return core.StatusPending, false
	case "FULL_REFUND", "PARTIAL_REFUND":
		return core.StatusRefunded, false
	case "CANCELED":
		return core.StatusCancelled, false
	case "AMBIGUOUS", "NOT_FOUND":
		return core.StatusFailed, false
	default:
		return core.StatusFailed, false
	}
}

// Ensure Provider implements core.Provider
var _ core.Provider = (*Provider)(nil)
