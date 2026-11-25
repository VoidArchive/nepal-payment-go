# Nepal Payment Go

A Go SDK for taking payments in Nepal. eSewa, Khalti, ConnectIPS and FonePay sit behind one interface.

[![Go Reference](https://pkg.go.dev/badge/github.com/voidarchive/nepal-payment-go/v2.svg)](https://pkg.go.dev/github.com/voidarchive/nepal-payment-go/v2)
[![CI](https://github.com/voidarchive/nepal-payment-go/actions/workflows/ci.yml/badge.svg)](https://github.com/voidarchive/nepal-payment-go/actions/workflows/ci.yml)

## Features

- One `Provider` interface: `InitiatePayment` and `VerifyPayment` work the same way for every gateway.
- `VerifyPayment` checks the gateway's signature, the amount and the order ID before it reports success.
- Failures wrap sentinel errors, so `errors.Is` can tell a tampered amount from a timeout.
- The library imports only the Go standard library. `testify` is used in tests.

## Supported providers

| Provider | Flow type | Authentication |
|----------|-----------|----------------|
| [eSewa](https://esewa.com.np) | Form POST | HMAC-SHA256 |
| [Khalti](https://khalti.com) | Redirect | API Key |
| [ConnectIPS](https://connectips.com) | Form POST | RSA-SHA256 Certificate |
| [FonePay](https://fonepay.com) | Redirect | HMAC-SHA512 |

## Installation

```bash
go get github.com/voidarchive/nepal-payment-go/v2
```

## Quick start

`Amount` is in paisa (1/100 rupee) for every provider, so Rs 100 is `10000`.

### eSewa (wallet)

```go
import "github.com/voidarchive/nepal-payment-go/v2/providers/esewa"

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

### Khalti (wallet)

```go
import "github.com/voidarchive/nepal-payment-go/v2/providers/khalti"

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

### ConnectIPS (bank transfer)

```go
import "github.com/voidarchive/nepal-payment-go/v2/providers/connectips"

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

### FonePay (bank or wallet)

```go
import "github.com/voidarchive/nepal-payment-go/v2/providers/fonepay"

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

### FonePay dynamic QR (in-person payments)

`GenerateQR` returns a QR code image for a customer to scan at the counter. It needs a separate API username and password, which your bank issues.

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

## Upgrading from v1

v2 changes the import path and a few behaviours:

- The module path ends in `/v2`.
- `Amount`, `ExpectedAmount` and `VerifiedAmount` are paisa for every provider. In v1, eSewa and FonePay took rupees, so multiply those amounts by 100.
- ConnectIPS takes a PEM key only. `CertPassword` and `CertFormat` are gone, and `VerifyPayment` needs `ExpectedAmount`.
- A callback for a different order than `OriginalOrderID` is rejected (eSewa, FonePay, ConnectIPS).
- The unused refund types, `ProviderInfo` and a few unused errors and fields were removed from `core`.

## Payment verification

Verification is the same call for every provider:

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

Two providers differ. Khalti's lookup does not return your order ID, so `OriginalOrderID` is not checked there: before you verify, match the callback `pidx` to the `TransactionID` that `InitiatePayment` returned for that order. ConnectIPS needs `ExpectedAmount` and returns an error when it is 0.

## Running the demo

The demo serves one checkout page for all four providers.

```bash
go run ./examples/demo/
```

Open http://localhost:8080

## Sandbox credentials

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

You onboard as a merchant with [Nepal Clearing House (NCHL)](https://nchl.com.np), which sends you a Merchant ID, an App ID, an app password and a `.pfx` certificate.

The SDK reads a PEM private key, so convert the `.pfx` once with OpenSSL. The command asks for the PFX password:

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

The FonePay sandbox is sometimes down. Production credentials come from FonePay.

## Environment variables

The demo reads these. `.env.example` has the same list.

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

## Project structure

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

## Core types

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

## Error handling

```go
import "github.com/voidarchive/nepal-payment-go/v2/core"

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

## Security notes

- Keep secret keys on the server. Anything sent to the browser can be read by the shopper.
- Call `VerifyPayment` on the server before you fulfil an order. A redirect to your success URL proves nothing, because the shopper controls it.
- Pass `ExpectedAmount` and `OriginalOrderID` from your own order record. Without them, a real payment for a cheaper order can be replayed against an expensive one.
- Use HTTPS callback URLs in production.
- `creditor.pem` has no password. Keep it out of git and run `chmod 600` on it.

## Contributing

Pull requests are welcome. Run `go test ./...` and `golangci-lint run ./...` before you open one.

## License

MIT. See [LICENSE](LICENSE).
