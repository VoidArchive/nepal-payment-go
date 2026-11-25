# Nepal Payment Go

A unified payment SDK for Nepal supporting multiple payment providers with a consistent API.

[![Go Reference](https://pkg.go.dev/badge/github.com/voidarchive/nepal-payment-go.svg)](https://pkg.go.dev/github.com/voidarchive/nepal-payment-go)
[![CI](https://github.com/voidarchive/nepal-payment-go/actions/workflows/ci.yml/badge.svg)](https://github.com/voidarchive/nepal-payment-go/actions/workflows/ci.yml)

## Features

- **Unified API** - Same interface for all providers
- **Type-safe** - Strongly typed requests and responses
- **Production-ready** - Proper error handling, timeouts, and logging
- **Well-tested** - Comprehensive test coverage
- **Zero dependencies** - Uses only the Go standard library

## Supported Providers

| Provider | Flow Type | Authentication | Status |
|----------|-----------|----------------|--------|
| [eSewa](https://esewa.com.np) | Form POST | HMAC-SHA256 | ✅ Ready |
| [Khalti](https://khalti.com) | Redirect | API Key | ✅ Ready |
| [ConnectIPS](https://connectips.com) | Form POST | RSA-SHA256 Certificate | ✅ Ready |
| [FonePay](https://fonepay.com) | Redirect | HMAC-SHA512 | ✅ Ready |

## Installation

```bash
go get github.com/voidarchive/nepal-payment-go
```

## Quick Start

`Amount` is always in paisa (1/100 rupee) for every provider: Rs 100 = `10000`. This changed for eSewa and FonePay, which used to take rupees.

### eSewa (Wallet)

```go
import "github.com/voidarchive/nepal-payment-go/providers/esewa"

provider, _ := esewa.New(esewa.Config{
    SecretKey:   "8gBm/:&EnhH.1/q",  // Sandbox key
    ProductCode: "EPAYTEST",
    Sandbox:     true,
})

// Initiate payment
resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      10000,  // In paisa (NPR 100)
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})

// resp.IsFormPOST = true
// Generate HTML form with resp.FormFields, POST to resp.TargetURL
```

### Khalti (Wallet)

```go
import "github.com/voidarchive/nepal-payment-go/providers/khalti"

provider, _ := khalti.New(khalti.Config{
    SecretKey:  os.Getenv("KHALTI_SECRET_KEY"),
    WebsiteURL: "https://yoursite.com",
    Sandbox:    true,
})

resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      10000,  // In paisa (NPR 100)
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})

// resp.IsFormPOST = false
// Redirect user to resp.TargetURL
```

### ConnectIPS (Bank Transfer)

```go
import "github.com/voidarchive/nepal-payment-go/providers/connectips"

provider, _ := connectips.New(connectips.Config{
    MerchantID:  os.Getenv("CONNECTIPS_MERCHANT_ID"),
    AppID:       os.Getenv("CONNECTIPS_APP_ID"),
    AppPassword: os.Getenv("CONNECTIPS_APP_PASSWORD"),
    CertPath:    "/path/to/creditor.pem", // converted from NCHL's .pfx, see below
    Sandbox:     true,
})

resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      10000,  // In paisa (NPR 100)
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})

// resp.IsFormPOST = true
// Generate HTML form with resp.FormFields, POST to resp.TargetURL
```

### FonePay (Bank/Wallet)

```go
import "github.com/voidarchive/nepal-payment-go/providers/fonepay"

provider, _ := fonepay.New(fonepay.Config{
    MerchantCode: "NBQM",  // Sandbox
    SecretKey:    "a7e3512f5032480a83137793cb2021dc",
    Sandbox:      true,
})

resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      10000,  // In paisa (NPR 100)
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})

// resp.IsFormPOST = false
// Redirect user to resp.TargetURL
```

### FonePay Dynamic QR (In-Person Payments)

Generate QR codes for retail/in-person payments. Requires additional credentials from your bank.

```go
provider, _ := fonepay.New(fonepay.Config{
    MerchantCode: "NBQM",
    SecretKey:    "a7e3512f5032480a83137793cb2021dc",
    Username:     os.Getenv("FONEPAY_USERNAME"),  // QR API credentials
    Password:     os.Getenv("FONEPAY_PASSWORD"),
    Sandbox:      true,
})

// Generate QR code
qr, _ := provider.GenerateQR(ctx, &fonepay.QRRequest{
    Amount:      500,  // QRRequest takes rupees (NPR 500), not paisa
    OrderID:     "TABLE-42",
    Description: "Coffee Shop Order",
})

// Display QR to customer
// qr.QRImageData contains PNG bytes
// Save to file: os.WriteFile("payment.png", qr.QRImageData, 0644)
```


## Payment Verification

All providers use the same verification pattern:

```go
result, err := provider.VerifyPayment(ctx, &core.VerifyRequest{
    EncodedParams:   callbackParams,  // Query params from callback
    OriginalOrderID: "ORDER-123",     // Callback for a different order is rejected
    ExpectedAmount:  10000,           // In paisa; 0 skips the check
})

if err != nil {
    if errors.Is(err, core.ErrAmountMismatch) {
        // Handle amount tampering
    }
    if errors.Is(err, core.ErrSignatureMismatch) {
        // Handle signature failure
    }
}

if result.Success {
    fmt.Printf("Payment complete! TxnID: %s", result.TransactionID)
}
```

Khalti does not check `OriginalOrderID`: match the callback `pidx` to the `TransactionID` returned by `InitiatePayment` yourself before verifying. ConnectIPS requires `ExpectedAmount` (it returns an error when it is 0).

## Running the Demo

### Combined Demo (All Providers)

```bash
go run ./examples/demo/
```

Open http://localhost:8080

## Sandbox Credentials

### eSewa

| Field | Value |
|-------|-------|
| Test ID | 9806800001 - 9806800005 |
| Password | Nepal@123 |
| Token | 123456 |
| Secret Key | `8gBm/:&EnhH.1/q` |
| Product Code | `EPAYTEST` |

### Khalti

| Field | Value |
|-------|-------|
| Test ID | 9800000001 - 9800000005 |
| MPIN | 1111 |
| OTP | 987654 |
| Secret Key | Get from [Khalti Dashboard](https://admin.khalti.com) |

### ConnectIPS

Requires merchant onboarding with [Nepal Clearing House (NCHL)](https://nchl.com.np).
You'll receive: Merchant ID, App ID, and a `.pfx` certificate.

The SDK reads a PEM private key, not the `.pfx`. Convert it once with OpenSSL (it asks for the PFX password):

```bash
openssl pkcs12 -in CREDITOR.pfx -nocerts -nodes -out creditor.pem
```

If OpenSSL 3 fails with an unsupported-algorithm error (older PFX files use RC2 encryption), add `-legacy`.

Then point `CertPath` (or `CONNECTIPS_CERT_PATH`) at `creditor.pem`. The PEM has no password, so keep it out of git and lock it down with `chmod 600 creditor.pem`.

### FonePay

| Field | Sandbox Value |
|-------|---------------|
| Merchant Code | `NBQM` |
| Secret Key | `a7e3512f5032480a83137793cb2021dc` |

Note: FonePay sandbox may be unreliable. Contact FonePay for production credentials.

## Environment Variables

```bash
# eSewa (optional - has sandbox defaults)
ESEWA_SECRET_KEY=8gBm/:&EnhH.1/q
ESEWA_PRODUCT_CODE=EPAYTEST

# Khalti (required)
KHALTI_SECRET_KEY=your_key_here

# ConnectIPS (required for ConnectIPS)
CONNECTIPS_MERCHANT_ID=your_merchant_id
CONNECTIPS_APP_ID=your_app_id
CONNECTIPS_APP_PASSWORD=your_password
CONNECTIPS_CERT_PATH=/path/to/creditor.pem

# FonePay (optional - has sandbox defaults)
FONEPAY_MERCHANT_CODE=NBQM
FONEPAY_SECRET_KEY=a7e3512f5032480a83137793cb2021dc
```

## Project Structure

```
.
├── core/                    # Core contracts (dependency-free)
│   ├── models.go           # Request/Response types
│   ├── errors.go           # Sentinel errors & status constants
│   ├── provider.go         # Provider interface
│   └── mock.go             # Mock provider for testing
├── providers/
│   ├── esewa/              # eSewa implementation
│   ├── khalti/             # Khalti implementation
│   ├── connectips/         # ConnectIPS implementation
│   └── fonepay/            # FonePay implementation
└── examples/
    └── demo/               # Combined demo (all providers)
```

## Core Types

```go
// PaymentRequest initiates a payment
type PaymentRequest struct {
    Amount        int64   // In paisa (Rs 100 = 10000)
    Currency      string  // Usually "NPR"
    OrderID       string  // Your unique order ID
    Description   string  // Payment description
    CallbackURL   string  // Where to redirect after payment
    CustomerEmail string  // Optional
    CustomerPhone string  // Optional
}

// PaymentResponse from InitiatePayment
type PaymentResponse struct {
    IsFormPOST    bool              // true = POST form, false = redirect
    TargetURL     string            // Where to send user
    FormFields    map[string]string // Form fields (if IsFormPOST)
    TransactionID string            // Provider's transaction ID (Khalti: pidx)
}

// VerifyRequest for VerifyPayment
type VerifyRequest struct {
    EncodedParams   map[string]string // Callback params
    OriginalOrderID string            // Rejects a callback for another order (not Khalti)
    ExpectedAmount  int64             // In paisa; 0 skips (required for ConnectIPS)
}

// VerifyResponse from VerifyPayment
type VerifyResponse struct {
    Success          bool
    TransactionID    string
    OrderID          string
    VerifiedAmount   int64             // In paisa
    Currency         string
    Status           string            // StatusComplete, StatusPending, etc.
    ProviderResponse map[string]string // Raw response for debugging
    VerifiedAt       int64             // Unix seconds
}
```

## Error Handling

```go
import "github.com/voidarchive/nepal-payment-go/core"

// Sentinel errors
core.ErrInvalidConfig    // Invalid provider configuration
core.ErrInvalidRequest   // Invalid payment request
core.ErrProviderError    // Provider returned an error
core.ErrAmountMismatch   // Verified amount doesn't match expected
core.ErrSignatureMismatch // Signature verification failed
core.ErrPaymentNotFound  // Transaction not found
core.ErrTimeout          // Request timed out
core.ErrRateLimited      // Rate limit exceeded

// Status constants
core.StatusComplete   // "COMPLETE"
core.StatusPending    // "PENDING"
core.StatusFailed     // "FAILED"
core.StatusCancelled  // "CANCELLED"
core.StatusExpired    // "EXPIRED"
core.StatusRefunded   // "REFUNDED"
```

## Testing

```bash
# Run all tests
go test ./... -v

# Run specific provider tests
go test ./providers/esewa/... -v
go test ./providers/khalti/... -v
go test ./providers/connectips/... -v
go test ./providers/fonepay/... -v

# Lint (same check CI runs)
golangci-lint run ./...
```

## Security Notes

1. **Never expose secret keys** in client-side code
2. **Always verify payments** server-side before fulfilling orders
3. **Validate amounts** to prevent price manipulation
4. **Use HTTPS** in production for callback URLs
5. **Store the ConnectIPS private key securely** (`creditor.pem` has no password: keep it out of git, `chmod 600`)

## Contributing

Contributions are welcome! Feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) file for details.
