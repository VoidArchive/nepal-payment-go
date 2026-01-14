package connectips

import (
	"bytes"
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
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/pkcs12"

	"github.com/voidarchive/nepal-payment-go/core"
)

const (
	sandboxLoginURL      = "https://uat.connectips.com/connectipswebgw/loginpage"
	sandboxValidationURL = "https://uat.connectips.com/connectipswebws/api/creditor/validatetxn"

	productionLoginURL      = "https://connectips.com/connectipswebgw/loginpage"
	productionValidationURL = "https://connectips.com/connectipswebws/api/creditor/validatetxn"
)

// Config holds ConnectIPS provider configuration.
type Config struct {
	MerchantID   string        // Creditor merchant ID from ConnectIPS
	AppID        string        // Application ID from ConnectIPS
	AppName      string        // Application name for display
	AppPassword  string        // Password for Basic Auth (API access)
	CertPath     string        // Path to .pfx or .pem certificate file
	CertPassword string        // Password for .pfx file (empty for .pem)
	CertData     []byte        // Raw certificate data (alternative to CertPath)
	CertFormat   string        // "pfx" or "pem" (required when using CertData)
	SuccessURL   string        // Redirect URL after successful payment
	FailureURL   string        // Redirect URL after failed payment
	Sandbox      bool          // Use sandbox environment
	Timeout      time.Duration // HTTP client timeout (default: 30s)
}

// Provider implements core.Provider for ConnectIPS payments.
type Provider struct {
	config        Config
	httpClient    *http.Client
	loginURL      string
	validationURL string
	privateKey    *rsa.PrivateKey
}

// New creates a new ConnectIPS provider.
func New(cfg Config) (*Provider, error) {
	if cfg.MerchantID == "" {
		return nil, fmt.Errorf("%w: MerchantID is required", core.ErrInvalidConfig)
	}
	if cfg.AppID == "" {
		return nil, fmt.Errorf("%w: AppID is required", core.ErrInvalidConfig)
	}
	if cfg.CertPath == "" && len(cfg.CertData) == 0 {
		return nil, fmt.Errorf("%w: CertPath or CertData is required", core.ErrInvalidConfig)
	}

	// Load certificate
	privateKey, err := loadPrivateKey(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load certificate: %v", core.ErrInvalidConfig, err)
	}

	loginURL := productionLoginURL
	validationURL := productionValidationURL
	if cfg.Sandbox {
		loginURL = sandboxLoginURL
		validationURL = sandboxValidationURL
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Provider{
		config:        cfg,
		loginURL:      loginURL,
		validationURL: validationURL,
		httpClient:    &http.Client{Timeout: timeout},
		privateKey:    privateKey,
	}, nil
}

// InitiatePayment creates form fields for ConnectIPS payment.
// Returns IsFormPOST=true with FormFields to POST to TargetURL.
func (p *Provider) InitiatePayment(ctx context.Context, req *core.PaymentRequest) (*core.PaymentResponse, error) {
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Current date in DD-MM-YYYY format
	txnDate := time.Now().Format("02-01-2006")

	// Generate token: sign the message with RSA-SHA256
	token, err := p.generateToken(req.OrderID, req.Amount, txnDate)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate token: %v", core.ErrProviderError, err)
	}

	successURL := p.config.SuccessURL
	if successURL == "" {
		successURL = req.CallbackURL
	}
	failureURL := p.config.FailureURL
	if failureURL == "" {
		failureURL = req.CallbackURL
	}

	// Build form fields for POST
	formFields := map[string]string{
		"MERCHANTID":  p.config.MerchantID,
		"APPID":       p.config.AppID,
		"APPNAME":     p.config.AppName,
		"TXNID":       req.OrderID,
		"TXNDATE":     txnDate,
		"TXNCRNCY":    "NPR",
		"TXNAMT":      strconv.FormatInt(req.Amount, 10),
		"REFERENCEID": req.OrderID,
		"REMARKS":     req.Description,
		"PARTICULARS": req.Description,
		"TOKEN":       token,
		"SUCCESSURL":  successURL,
		"FAILUREURL":  failureURL,
	}

	return &core.PaymentResponse{
		IsFormPOST:    true,
		TargetURL:     p.loginURL,
		FormFields:    formFields,
		TransactionID: req.OrderID,
	}, nil
}

// VerifyPayment validates callback data and confirms payment status.
// Expects EncodedParams["TXNID"] to contain the transaction ID from callback.
func (p *Provider) VerifyPayment(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	// Get transaction ID from callback
	txnID := req.EncodedParams["TXNID"]
	if txnID == "" {
		txnID = req.OriginalOrderID
	}
	if txnID == "" {
		return nil, fmt.Errorf("%w: TXNID not found in callback", core.ErrInvalidRequest)
	}

	// Check callback status first
	callbackStatus := req.EncodedParams["STATUS"]
	if callbackStatus == "FAILED" || callbackStatus == "CANCELLED" {
		return &core.VerifyResponse{
			Success:       false,
			TransactionID: req.EncodedParams["TRANSACTIONREFID"],
			OrderID:       txnID,
			Status:        p.normalizeStatus(callbackStatus),
			ProviderResponse: map[string]string{
				"status":     callbackStatus,
				"status_msg": req.EncodedParams["STATUS_MSG"],
			},
			VerifiedAt: time.Now().Unix(),
		}, nil
	}

	// Validate transaction via API
	return p.validateTransaction(ctx, txnID, req.ExpectedAmount)
}

func (p *Provider) validateTransaction(ctx context.Context, txnID string, expectedAmount int64) (*core.VerifyResponse, error) {
	// Generate validation token
	token, err := p.generateValidationToken(txnID, expectedAmount)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate validation token: %v", core.ErrProviderError, err)
	}

	payload := validationRequest{
		MerchantID:  p.config.MerchantID,
		AppID:       p.config.AppID,
		ReferenceID: txnID,
		TxnAmount:   expectedAmount,
		Token:       token,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal request: %v", core.ErrProviderError, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.validationURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %v", core.ErrProviderError, err)
	}

	// Basic auth with AppID:AppPassword
	httpReq.SetBasicAuth(p.config.AppID, p.config.AppPassword)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", core.ErrTimeout, ctx.Err())
		}
		return nil, fmt.Errorf("%w: request failed: %v", core.ErrProviderError, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %v", core.ErrProviderError, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", core.ErrProviderError, resp.StatusCode, string(respBody))
	}

	var valResp validationResponse
	if err := json.Unmarshal(respBody, &valResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse response: %v", core.ErrProviderError, err)
	}

	// Check for NOT_FOUND or INVALID_TOKEN
	normalizedStatus := p.normalizeStatus(valResp.Status)
	if valResp.Status == statusNotFound {
		return nil, core.ErrPaymentNotFound
	}
	if valResp.Status == statusInvalidToken || !valResp.TokenValid {
		return nil, fmt.Errorf("%w: invalid token", core.ErrSignatureMismatch)
	}

	// Validate amount
	if expectedAmount > 0 && expectedAmount != valResp.TxnAmount {
		return nil, fmt.Errorf("%w: expected %d, got %d",
			core.ErrAmountMismatch, expectedAmount, valResp.TxnAmount)
	}

	success := valResp.Status == statusSuccess

	return &core.VerifyResponse{
		Success:        success,
		TransactionID:  valResp.TransRefID,
		OrderID:        valResp.ReferenceID,
		VerifiedAmount: valResp.TxnAmount,
		Currency:       "NPR",
		Status:         normalizedStatus,
		ProviderResponse: map[string]string{
			"status":       valResp.Status,
			"status_desc":  valResp.StatusDesc,
			"trans_ref_id": valResp.TransRefID,
			"reference_id": valResp.ReferenceID,
			"txn_date":     valResp.TxnDate,
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

// generateToken creates the signed token for payment initiation.
// Message format: MERCHANTID,APPID,RETEFID,TXNAMT,TXNID
func (p *Provider) generateToken(txnID string, amount int64, txnDate string) (string, error) {
	message := fmt.Sprintf("%s,%s,%s,%d,%s",
		p.config.MerchantID,
		p.config.AppID,
		txnID,
		amount,
		txnDate,
	)
	return p.signMessage(message)
}

// generateValidationToken creates the signed token for validation API.
func (p *Provider) generateValidationToken(txnID string, amount int64) (string, error) {
	message := fmt.Sprintf("%s,%s,%s,%d",
		p.config.MerchantID,
		p.config.AppID,
		txnID,
		amount,
	)
	return p.signMessage(message)
}

// signMessage signs a message with SHA256withRSA and returns Base64-encoded signature.
func (p *Provider) signMessage(message string) (string, error) {
	hash := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (p *Provider) normalizeStatus(status string) string {
	switch strings.ToUpper(status) {
	case statusSuccess:
		return core.StatusComplete
	case statusPending:
		return core.StatusPending
	case statusCancelled:
		return core.StatusCancelled
	case statusTimeout, statusSessionExpired:
		return core.StatusExpired
	default:
		return core.StatusFailed
	}
}

// loadPrivateKey loads the RSA private key from PFX or PEM file/data.
func loadPrivateKey(cfg Config) (*rsa.PrivateKey, error) {
	var certData []byte
	var err error
	var isPFX bool

	if len(cfg.CertData) > 0 {
		certData = cfg.CertData
		// Use CertFormat to determine format when using raw data
		isPFX = strings.EqualFold(cfg.CertFormat, "pfx") || strings.EqualFold(cfg.CertFormat, "p12")
	} else {
		certData, err = os.ReadFile(cfg.CertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}
		// Detect format from file extension
		lowerPath := strings.ToLower(cfg.CertPath)
		isPFX = strings.HasSuffix(lowerPath, ".pfx") || strings.HasSuffix(lowerPath, ".p12")
	}

	if isPFX {
		return loadFromPFX(certData, cfg.CertPassword)
	}

	return loadFromPEM(certData)
}

func loadFromPFX(data []byte, password string) (*rsa.PrivateKey, error) {
	privateKey, _, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PFX: %w", err)
	}

	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("certificate does not contain RSA private key")
	}

	return rsaKey, nil
}

func loadFromPEM(data []byte) (*rsa.PrivateKey, error) {
	// Decode PEM block first
	block, _ := pem.Decode(data)
	if block == nil {
		// Not PEM encoded, try raw DER
		return parseDERKey(data)
	}

	return parseDERKey(block.Bytes)
}

func parseDERKey(data []byte) (*rsa.PrivateKey, error) {
	// Try PKCS8 format first
	key, err := x509.ParsePKCS8PrivateKey(data)
	if err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key is not RSA private key")
		}
		return rsaKey, nil
	}

	// Try PKCS1 format
	rsaKey, err := x509.ParsePKCS1PrivateKey(data)
	if err == nil {
		return rsaKey, nil
	}

	return nil, fmt.Errorf("failed to parse private key: not PKCS8 or PKCS1 format")
}

// Ensure Provider implements core.Provider
var _ core.Provider = (*Provider)(nil)
