# Nepal Payment Go

A unified payment SDK for Nepal supporting multiple payment providers with a consistent API.

[![Go Reference](https://pkg.go.dev/badge/github.com/voidarchive/nepal-payment-go.svg)](https://pkg.go.dev/github.com/voidarchive/nepal-payment-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/voidarchive/nepal-payment-go)](https://goreportcard.com/report/github.com/voidarchive/nepal-payment-go)

## Features

- **Unified API** - Same interface for all providers
- **Type-safe** - Strongly typed requests and responses
- **Production-ready** - Proper error handling, timeouts, and logging
- **Well-tested** - Comprehensive test coverage
- **Zero dependencies** - Uses only Go standard library (except `golang.org/x/crypto` for ConnectIPS)

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
    Amount:      100,
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
    MerchantID:   os.Getenv("CONNECTIPS_MERCHANT_ID"),
    AppID:        os.Getenv("CONNECTIPS_APP_ID"),
    AppPassword:  os.Getenv("CONNECTIPS_APP_PASSWORD"),
    CertPath:     "/path/to/creditor.pfx",
    CertPassword: os.Getenv("CONNECTIPS_CERT_PASSWORD"),
    Sandbox:      true,
})

resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      100,
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
    Amount:      100,
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})

// resp.IsFormPOST = false
// Redirect user to resp.TargetURL
```

## Payment Verification

All providers use the same verification pattern:

```go
result, err := provider.VerifyPayment(ctx, &core.VerifyRequest{
    EncodedParams:  callbackParams,  // Query params from callback
    ExpectedAmount: 100,
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

## Running the Demo

### Combined Demo (All Providers)

```bash
go run ./examples/demo/
```

Open http://localhost:8080

### Individual Provider Demos

```bash
go run ./examples/esewa/      # eSewa demo
go run ./examples/khalti/     # Khalti demo (needs KHALTI_SECRET_KEY)
go run ./examples/connectips/ # ConnectIPS demo (needs credentials)
go run ./examples/fonepay/    # FonePay demo
```

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
You'll receive: Merchant ID, App ID, and `.pfx` certificate.

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
CONNECTIPS_CERT_PATH=/path/to/creditor.pfx
CONNECTIPS_CERT_PASSWORD=cert_password

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
    ├── demo/               # Combined demo (all providers)
    ├── esewa/              # eSewa standalone demo
    ├── khalti/             # Khalti standalone demo
    ├── connectips/         # ConnectIPS standalone demo
    └── fonepay/            # FonePay standalone demo
```

## Core Types

```go
// PaymentRequest initiates a payment
type PaymentRequest struct {
    Amount        int64   // Amount in smallest unit
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
    TransactionID string            // Provider's transaction ID
}

// VerifyResponse from VerifyPayment
type VerifyResponse struct {
    Success        bool
    TransactionID  string
    OrderID        string
    VerifiedAmount int64
    Currency       string
    Status         string  // StatusComplete, StatusPending, etc.
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
core.StatusComplete   // "complete"
core.StatusPending    // "pending"
core.StatusFailed     // "failed"
core.StatusCancelled  // "cancelled"
core.StatusExpired    // "expired"
core.StatusRefunded   // "refunded"
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
```

## Security Notes

1. **Never expose secret keys** in client-side code
2. **Always verify payments** server-side before fulfilling orders
3. **Validate amounts** to prevent price manipulation
4. **Use HTTPS** in production for callback URLs
5. **Store certificates securely** (ConnectIPS .pfx files)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) file for details.
