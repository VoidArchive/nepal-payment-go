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
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/voidarchive/nepal-payment-go/v2/core"
)

const (
	sandboxLoginURL      = "https://uat.connectips.com/connectipswebgw/loginpage"
	sandboxValidationURL = "https://uat.connectips.com/connectipswebws/api/creditor/validatetxn"

	productionLoginURL      = "https://connectips.com/connectipswebgw/loginpage"
	productionValidationURL = "https://connectips.com/connectipswebws/api/creditor/validatetxn"
)

// Config holds ConnectIPS provider configuration.
type Config struct {
	MerchantID  string        // Creditor merchant ID from ConnectIPS
	AppID       string        // Application ID from ConnectIPS
	AppName     string        // Application name for display
	AppPassword string        // Password for Basic Auth (API access)
	CertPath    string        // Path to the PEM (or DER) RSA private key; convert NCHL's .pfx first (see README)
	CertData    []byte        // Raw PEM (or DER) private key (alternative to CertPath)
	SuccessURL  string        // Redirect URL after successful payment
	FailureURL  string        // Redirect URL after failed payment
	Sandbox     bool          // Use sandbox environment
	Timeout     time.Duration // HTTP client timeout (default: 30s)
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
	if ext := strings.ToLower(filepath.Ext(cfg.CertPath)); ext == ".pfx" || ext == ".p12" {
		return nil, fmt.Errorf("%w: CertPath %q is a PKCS#12 file; convert it to a PEM private key once (see README, ConnectIPS)", core.ErrInvalidConfig, cfg.CertPath)
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
		"TXNDATE":     time.Now().Format("02-01-2006"),
		"TXNCRNCY":    "NPR",
		"TXNAMT":      strconv.FormatInt(req.Amount, 10),
		"REFERENCEID": req.OrderID,
		"REMARKS":     req.Description,
		"PARTICULARS": req.Description,
		"SUCCESSURL":  successURL,
		"FAILUREURL":  failureURL,
	}

	token, err := p.generateToken(formFields)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate token: %v", core.ErrProviderError, err)
	}
	formFields["TOKEN"] = token

	return &core.PaymentResponse{
		IsFormPOST:    true,
		TargetURL:     p.loginURL,
		FormFields:    formFields,
		TransactionID: req.OrderID,
	}, nil
}

// VerifyPayment validates callback data and confirms payment status.
// Expects EncodedParams["TXNID"] to contain the transaction ID from callback.
// connectIPS appends only TXNID to the redirect URL, so the result always comes from the validation API.
func (p *Provider) VerifyPayment(ctx context.Context, req *core.VerifyRequest) (*core.VerifyResponse, error) {
	callbackTxnID := req.EncodedParams["TXNID"]
	if req.OriginalOrderID != "" && callbackTxnID != "" && req.OriginalOrderID != callbackTxnID {
		return nil, fmt.Errorf("%w: order ID mismatch: expected %s, got %s",
			core.ErrInvalidRequest, req.OriginalOrderID, callbackTxnID)
	}
	txnID := callbackTxnID
	if txnID == "" {
		txnID = req.OriginalOrderID
	}
	if txnID == "" {
		return nil, fmt.Errorf("%w: TXNID not found in callback", core.ErrInvalidRequest)
	}
	// The validation API signs and checks the amount, so it cannot be skipped.
	if req.ExpectedAmount <= 0 {
		return nil, fmt.Errorf("%w: ExpectedAmount is required for ConnectIPS", core.ErrInvalidRequest)
	}

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
	if valResp.Status == statusInvalidToken {
		return nil, fmt.Errorf("%w: invalid token", core.ErrSignatureMismatch)
	}

	// A failed reply may omit referenceId and txnAmt; only a present value can mismatch.
	if valResp.ReferenceID != "" && valResp.ReferenceID != txnID {
		return nil, fmt.Errorf("%w: order ID mismatch: expected %s, got %s",
			core.ErrInvalidRequest, txnID, valResp.ReferenceID)
	}

	// txnAmt is paisa, sent either as a string ("500") or a number (1000.00).
	success := valResp.Status == statusSuccess
	var verifiedAmount int64
	if success || valResp.TxnAmt != "" {
		amount, err := strconv.ParseFloat(valResp.TxnAmt.String(), 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid txnAmt %q: %v", core.ErrProviderError, valResp.TxnAmt, err)
		}
		verifiedAmount = int64(math.Round(amount))
		if expectedAmount != verifiedAmount {
			return nil, fmt.Errorf("%w: expected %d, got %d",
				core.ErrAmountMismatch, expectedAmount, verifiedAmount)
		}
	}

	return &core.VerifyResponse{
		Success:        success,
		TransactionID:  "", // the validation reply carries no connectIPS transaction ID
		OrderID:        txnID,
		VerifiedAmount: verifiedAmount,
		Currency:       "NPR",
		Status:         normalizedStatus,
		ProviderResponse: map[string]string{
			"status":       valResp.Status,
			"status_desc":  valResp.StatusDesc,
			"reference_id": valResp.ReferenceID,
			"txn_amt":      valResp.TxnAmt.String(),
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

// tokenFields is the order connectIPS signs the initiation form fields in.
var tokenFields = []string{"MERCHANTID", "APPID", "APPNAME", "TXNID", "TXNDATE", "TXNCRNCY", "TXNAMT", "REFERENCEID", "REMARKS", "PARTICULARS"}

// generateToken signs the initiation form fields, so the signed values are the posted ones.
// Message format: MERCHANTID=<v>,APPID=<v>,...,PARTICULARS=<v>,TOKEN=TOKEN
func (p *Provider) generateToken(fields map[string]string) (string, error) {
	parts := make([]string, 0, len(tokenFields)+1)
	for _, k := range tokenFields {
		parts = append(parts, k+"="+fields[k])
	}
	return p.signMessage(strings.Join(append(parts, "TOKEN=TOKEN"), ","))
}

// generateValidationToken creates the signed token for validation API.
func (p *Provider) generateValidationToken(txnID string, amount int64) (string, error) {
	return p.signMessage(fmt.Sprintf("MERCHANTID=%s,APPID=%s,REFERENCEID=%s,TXNAMT=%d",
		p.config.MerchantID, p.config.AppID, txnID, amount))
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

// loadPrivateKey loads the RSA private key from a PEM (or DER) file or data.
func loadPrivateKey(cfg Config) (*rsa.PrivateKey, error) {
	if len(cfg.CertData) > 0 {
		return loadFromPEM(cfg.CertData)
	}
	certData, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}
	return loadFromPEM(certData)
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
