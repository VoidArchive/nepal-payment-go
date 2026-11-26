package khalti

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/voidarchive/nepal-payment-go/v2/core"
)

const (
	sandboxBaseURL    = "https://dev.khalti.com/api/v2"
	productionBaseURL = "https://khalti.com/api/v2"

	initiateEndpoint = "/epayment/initiate/"
	lookupEndpoint   = "/epayment/lookup/"
)

// Config holds Khalti provider configuration.
type Config struct {
	SecretKey  string        // Live secret key from Khalti dashboard
	WebsiteURL string        // Your website URL (required by Khalti)
	Sandbox    bool          // Use sandbox environment
	Timeout    time.Duration // HTTP client timeout (default: 30s)
	BaseURL    string        // Override API base URL (for testing)
}

// Provider implements core.Provider for Khalti payments.
type Provider struct {
	config     Config
	httpClient *http.Client
	baseURL    string
}

// New creates a new Khalti provider.
func New(cfg Config) (*Provider, error) {
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("%w: SecretKey is required", core.ErrInvalidConfig)
	}
	if cfg.WebsiteURL == "" {
		return nil, fmt.Errorf("%w: WebsiteURL is required", core.ErrInvalidConfig)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = productionBaseURL
		if cfg.Sandbox {
			baseURL = sandboxBaseURL
		}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Provider{
		config:     cfg,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// InitiatePayment creates a new payment session with Khalti.
func (p *Provider) InitiatePayment(ctx context.Context, req *core.PaymentRequest) (*core.PaymentResponse, error) {
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	payload := initiateRequest{
		ReturnURL:         req.CallbackURL,
		WebsiteURL:        p.config.WebsiteURL,
		Amount:            req.Amount,
		PurchaseOrderID:   req.OrderID,
		PurchaseOrderName: req.Description,
	}

	// Add customer info if available
	if req.CustomerEmail != "" || req.CustomerPhone != "" {
		payload.CustomerInfo = &customerInfo{
			Email: req.CustomerEmail,
			Phone: req.CustomerPhone,
		}
	}

	var resp initiateResponse
	if err := p.doRequest(ctx, initiateEndpoint, payload, &resp); err != nil {
		return nil, err
	}

	return &core.PaymentResponse{
		IsFormPOST:    false, // Khalti uses redirect
		TargetURL:     resp.PaymentURL,
		TransactionID: resp.PIDX,
	}, nil
}

// VerifyPayment validates a callback and confirms payment status.
// Khalti's lookup does not return the order ID, so OriginalOrderID is not checked: set
// ProviderRef to the TransactionID (pidx) that InitiatePayment returned for that order.
func (p *Provider) VerifyPayment(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	// Get pidx from callback params
	pidx := req.EncodedParams["pidx"]
	if pidx == "" {
		return nil, fmt.Errorf("%w: pidx not found in callback", core.ErrInvalidRequest)
	}
	if req.ProviderRef != "" && req.ProviderRef != pidx {
		return nil, fmt.Errorf("%w: pidx mismatch: expected %s, got %s", core.ErrInvalidRequest, req.ProviderRef, pidx)
	}
	return p.lookup(ctx, pidx, req.ExpectedAmount, req.OriginalOrderID)
}

// CheckStatus looks up a payment by its pidx (StatusRequest.ProviderRef) without callback data.
func (p *Provider) CheckStatus(ctx context.Context, req *core.StatusRequest) (*core.VerifyResponse, error) {
	if req.ProviderRef == "" {
		return nil, fmt.Errorf("%w: ProviderRef (pidx) is required", core.ErrInvalidRequest)
	}
	return p.lookup(ctx, req.ProviderRef, req.Amount, req.OrderID)
}

// lookup asks Khalti for the payment's status. expectedAmount 0 skips the amount check.
func (p *Provider) lookup(ctx context.Context, pidx string, expectedAmount int64, orderID string) (*core.VerifyResponse, error) {
	payload := lookupRequest{PIDX: pidx}

	var resp lookupResponse
	// Khalti answers cancelled and expired payments with HTTP 400 and the normal lookup body.
	if err := p.doRequest(ctx, lookupEndpoint, payload, &resp); err != nil && resp.Status == "" {
		return nil, err
	}

	// Normalize status
	status, success := p.normalizeStatus(resp.Status)

	// Validate amount if expected amount is provided
	if expectedAmount > 0 && expectedAmount != resp.TotalAmount {
		return nil, fmt.Errorf("%w: expected %d, got %d", core.ErrAmountMismatch, expectedAmount, resp.TotalAmount)
	}

	return &core.VerifyResponse{
		Success:        success,
		TransactionID:  resp.TransactionID,
		OrderID:        orderID,
		VerifiedAmount: resp.TotalAmount,
		Currency:       "NPR", // Khalti only supports NPR
		Status:         status,
		ProviderResponse: map[string]string{
			"pidx":     resp.PIDX,
			"status":   resp.Status,
			"refunded": fmt.Sprintf("%t", resp.Refunded),
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
	if req.CallbackURL == "" {
		return fmt.Errorf("%w: CallbackURL is required", core.ErrInvalidRequest)
	}
	return nil
}

func (p *Provider) doRequest(ctx context.Context, endpoint string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal request: %v", core.ErrProviderError, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: failed to create request: %v", core.ErrProviderError, err)
	}

	req.Header.Set("Authorization", "Key "+p.config.SecretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w: %v", core.ErrTimeout, ctx.Err())
		}
		return fmt.Errorf("%w: request failed: %v", core.ErrProviderError, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: failed to read response: %v", core.ErrProviderError, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = json.Unmarshal(respBody, result) // some error replies carry a usable body; see VerifyPayment
		return p.handleErrorResponse(resp.StatusCode, respBody)
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("%w: failed to parse response: %v", core.ErrProviderError, err)
	}

	return nil
}

func (p *Provider) handleErrorResponse(statusCode int, body []byte) error {
	var err error
	switch statusCode {
	case http.StatusUnauthorized:
		err = fmt.Errorf("%w: invalid secret key", core.ErrInvalidConfig)
	case http.StatusBadRequest:
		err = fmt.Errorf("%w: bad request", core.ErrInvalidRequest)
	case http.StatusNotFound:
		err = core.ErrPaymentNotFound
	case http.StatusTooManyRequests:
		err = core.ErrRateLimited
	default:
		err = fmt.Errorf("%w: HTTP %d", core.ErrProviderError, statusCode)
	}

	var errResp errorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Detail != "" {
		return fmt.Errorf("%w: %s", err, errResp.Detail)
	}
	// Validation errors have no "detail": {"purchase_order_name":["This field may not be blank."],"error_key":"validation_error"}.
	if statusCode == http.StatusBadRequest && len(body) > 0 {
		return fmt.Errorf("%w: %s", err, body[:min(len(body), 500)])
	}
	return err
}

func (p *Provider) normalizeStatus(khaltiStatus string) (string, bool) {
	switch khaltiStatus {
	case "Completed":
		return core.StatusComplete, true
	case "Pending", "Initiated":
		return core.StatusPending, false
	case "Refunded", "Partially refunded":
		return core.StatusRefunded, false
	case "Expired":
		return core.StatusExpired, false
	case "User canceled":
		return core.StatusCancelled, false
	default:
		return core.StatusFailed, false
	}
}

// Ensure Provider implements core.Provider
var (
	_ core.Provider      = (*Provider)(nil)
	_ core.StatusChecker = (*Provider)(nil)
)
