package fonepay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/voidarchive/nepal-payment-go/core"
)

const (
	sandboxBaseURL    = "https://dev-clientapi.fonepay.com/api/merchantRequest"
	productionBaseURL = "https://clientapi.fonepay.com/api/merchantRequest"

	// QR API endpoints
	sandboxQRURL    = "https://dev-merchantapi.fonepay.com/api/merchant/merchantDetailsForThirdParty/thirdPartyDynamicQrDownload"
	productionQRURL = "https://merchantapi.fonepay.com/api/merchant/merchantDetailsForThirdParty/thirdPartyDynamicQrDownload"
)

// Config holds FonePay provider configuration.
type Config struct {
	MerchantCode string        // Merchant code (PID) from FonePay
	SecretKey    string        // HMAC secret key from FonePay
	Sandbox      bool          // Use sandbox environment
	Timeout      time.Duration // HTTP client timeout (default: 30s)

	// QR API credentials (optional, only needed for GenerateQR)
	Username string // FonePay merchant API username
	Password string // FonePay merchant API password
}

// Provider implements core.Provider for FonePay payments.
type Provider struct {
	config     Config
	httpClient *http.Client
	baseURL    string
	qrAPIURL   string
}

// New creates a new FonePay provider.
func New(cfg Config) (*Provider, error) {
	if cfg.MerchantCode == "" {
		return nil, fmt.Errorf("%w: MerchantCode is required", core.ErrInvalidConfig)
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("%w: SecretKey is required", core.ErrInvalidConfig)
	}

	baseURL := productionBaseURL
	qrURL := productionQRURL
	if cfg.Sandbox {
		baseURL = sandboxBaseURL
		qrURL = sandboxQRURL
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Provider{
		config:     cfg,
		baseURL:    baseURL,
		qrAPIURL:   qrURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// InitiatePayment creates the redirect URL for FonePay payment.
// Returns IsFormPOST=false with TargetURL for redirect.
func (p *Provider) InitiatePayment(ctx context.Context, req *core.PaymentRequest) (*core.PaymentResponse, error) {
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Current date in format MM/DD/YYYY
	dt := time.Now().Format("01/02/2006")

	// Build payment parameters
	params := url.Values{}
	params.Set("PID", p.config.MerchantCode)
	params.Set("MD", "P")                                       // Payment mode
	params.Set("PRN", req.OrderID)                              // Product Reference Number
	params.Set("AMT", fmt.Sprintf("%.2f", float64(req.Amount))) // Amount as decimal
	params.Set("CRN", "NPR")                                    // Currency
	params.Set("DT", dt)                                        // Date
	params.Set("R1", req.Description)                           // Remarks 1
	params.Set("R2", "N/A")                                     // Remarks 2
	params.Set("RU", req.CallbackURL)                           // Return URL

	// Generate HMAC-SHA512 hash (DV = Data Validation)
	// Message format: PID,MD,PRN,AMT,CRN,DT,R1,R2,RU
	message := fmt.Sprintf("%s,%s,%s,%.2f,%s,%s,%s,%s,%s",
		p.config.MerchantCode,
		"P",
		req.OrderID,
		float64(req.Amount),
		"NPR",
		dt,
		req.Description,
		"N/A",
		req.CallbackURL,
	)
	dv := p.generateHMAC(message)
	params.Set("DV", dv)

	targetURL := p.baseURL + "?" + params.Encode()

	return &core.PaymentResponse{
		IsFormPOST:    false, // FonePay uses redirect
		TargetURL:     targetURL,
		TransactionID: req.OrderID,
	}, nil
}

// VerifyPayment validates callback data and confirms payment status.
// Expects EncodedParams from the callback query string.
func (p *Provider) VerifyPayment(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	// Extract callback parameters
	prn := req.EncodedParams["PRN"]
	if prn == "" {
		prn = req.OriginalOrderID
	}
	if prn == "" {
		return nil, fmt.Errorf("%w: PRN not found in callback", core.ErrInvalidRequest)
	}

	// Get all callback params
	bid := req.EncodedParams["BID"]
	uid := req.EncodedParams["UID"]
	ps := req.EncodedParams["PS"]
	rc := req.EncodedParams["RC"]
	dv := req.EncodedParams["DV"]
	status := req.EncodedParams["P"]

	// Verify HMAC signature
	// Response DV format: PRN,BID,UID,PS,RC,DV is calculated from PRN,BID,UID
	if dv != "" {
		message := fmt.Sprintf("%s,%s,%s", prn, bid, uid)
		expectedDV := p.generateHMAC(message)
		if !hmac.Equal([]byte(strings.ToLower(expectedDV)), []byte(strings.ToLower(dv))) {
			return nil, fmt.Errorf("%w: HMAC signature mismatch", core.ErrSignatureMismatch)
		}
	}

	// Normalize status
	normalizedStatus, success := p.normalizeStatus(status, rc)

	// Parse amount if provided
	var verifiedAmount int64
	if amtStr := req.EncodedParams["AMT"]; amtStr != "" {
		if amt, err := strconv.ParseInt(amtStr, 10, 64); err == nil {
			verifiedAmount = amt
		}
	}

	// Validate amount if expected
	if req.ExpectedAmount > 0 && verifiedAmount > 0 && req.ExpectedAmount != verifiedAmount {
		return nil, fmt.Errorf("%w: expected %d, got %d",
			core.ErrAmountMismatch, req.ExpectedAmount, verifiedAmount)
	}

	return &core.VerifyResponse{
		Success:        success,
		TransactionID:  uid,
		OrderID:        prn,
		VerifiedAmount: verifiedAmount,
		Currency:       "NPR",
		Status:         normalizedStatus,
		ProviderResponse: map[string]string{
			"prn": prn,
			"bid": bid,
			"uid": uid,
			"ps":  ps,
			"rc":  rc,
			"p":   status,
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

// generateHMAC creates HMAC-SHA512 hash and returns hex-encoded string.
func (p *Provider) generateHMAC(message string) string {
	mac := hmac.New(sha512.New, []byte(p.config.SecretKey))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func (p *Provider) normalizeStatus(status string, rc string) (string, bool) {
	// FonePay returns P=true for success
	if strings.ToLower(status) == "true" {
		return core.StatusComplete, true
	}

	// Check response code for more detailed status
	switch rc {
	case "successful", "success":
		return core.StatusComplete, true
	case "pending":
		return core.StatusPending, false
	case "cancelled", "canceled":
		return core.StatusCancelled, false
	default:
		return core.StatusFailed, false
	}
}

// GenerateQR creates a dynamic QR code for in-person payments.
// Requires Username and Password to be configured.
// Returns QR image data that can be displayed to customers.
func (p *Provider) GenerateQR(ctx context.Context, req *QRRequest) (*QRResponse, error) {
	if err := p.validateQRConfig(); err != nil {
		return nil, err
	}
	if err := p.validateQRRequest(req); err != nil {
		return nil, err
	}

	// Build data validation hash
	// Format: amount,prn,merchantCode,remarks1,remarks2
	remarks2 := req.Remarks2
	if remarks2 == "" {
		remarks2 = "N/A"
	}

	message := fmt.Sprintf("%d,%s,%s,%s,%s",
		req.Amount,
		req.OrderID,
		p.config.MerchantCode,
		req.Description,
		remarks2,
	)
	dataValidation := p.generateHMAC(message)

	// Build request payload
	payload := qrAPIRequest{
		Amount:         strconv.FormatInt(req.Amount, 10),
		PRN:            req.OrderID,
		MerchantCode:   p.config.MerchantCode,
		Remarks1:       req.Description,
		Remarks2:       remarks2,
		DataValidation: dataValidation,
		Username:       p.config.Username,
		Password:       p.config.Password,
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal request: %v", core.ErrProviderError, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.qrAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %v", core.ErrProviderError, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("%w: HTTP %d: %s", core.ErrProviderError, resp.StatusCode, string(body))
	}

	var qrResp qrAPIResponse
	if err := json.Unmarshal(body, &qrResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse response: %v", core.ErrProviderError, err)
	}

	if !qrResp.Success {
		return nil, fmt.Errorf("%w: %s", core.ErrProviderError, qrResp.Message)
	}

	// Decode Base64 QR image
	qrImageData, err := base64.StdEncoding.DecodeString(qrResp.QRCode)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode QR image: %v", core.ErrProviderError, err)
	}

	return &QRResponse{
		QRImageData:   qrImageData,
		TransactionID: qrResp.PRN,
		MerchantCode:  qrResp.MerchantCode,
	}, nil
}

func (p *Provider) validateQRConfig() error {
	if p.config.Username == "" {
		return fmt.Errorf("%w: Username is required for QR API", core.ErrInvalidConfig)
	}
	if p.config.Password == "" {
		return fmt.Errorf("%w: Password is required for QR API", core.ErrInvalidConfig)
	}
	return nil
}

func (p *Provider) validateQRRequest(req *QRRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", core.ErrInvalidRequest)
	}
	if req.OrderID == "" {
		return fmt.Errorf("%w: OrderID is required", core.ErrInvalidRequest)
	}
	if req.Description == "" {
		return fmt.Errorf("%w: Description is required", core.ErrInvalidRequest)
	}
	return nil
}

// Ensure Provider implements core.Provider
var _ core.Provider = (*Provider)(nil)
