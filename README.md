# Nepal Payment Go

This Go SDK lets your application receive payments in Nepal. It supports eSewa, Khalti, ConnectIPS and FonePay. All four providers use the same interface.

[![Go Reference](https://pkg.go.dev/badge/github.com/voidarchive/nepal-payment-go/v2.svg)](https://pkg.go.dev/github.com/voidarchive/nepal-payment-go/v2)
[![CI](https://github.com/voidarchive/nepal-payment-go/actions/workflows/ci.yml/badge.svg)](https://github.com/voidarchive/nepal-payment-go/actions/workflows/ci.yml)

## Features

- There is one `Provider` interface. `InitiatePayment` and `VerifyPayment` operate the same for each provider.
- `VerifyPayment` examines the signature, the amount and the order ID. Then it reports the result.
- Each error wraps a sentinel error. Use `errors.Is` to find the cause of an error.
- The `payments` package keeps payment records and finds payments that the customer did not complete in the browser.
- The library imports only the Go standard library. Only the tests use `testify`.

## Supported providers

| Provider | Flow type | Authentication |
|----------|-----------|----------------|
| [eSewa](https://esewa.com.np) | Form POST | HMAC-SHA256 |
| [Khalti](https://khalti.com) | Redirect | API Key |
| [ConnectIPS](https://connectips.com) | Form POST | RSA-SHA256 Certificate |
| [FonePay](https://fonepay.com) | Redirect | HMAC-SHA512 |

### Test status

- eSewa: tested with a real payment in the eSewa sandbox.
- Khalti: tested in the Khalti sandbox with version 1. Version 2 is not tested again.
- FonePay: the request and the callback signature are tested in the FonePay sandbox. A successful payment is not tested, because the sandbox bank pages do not work.
- ConnectIPS: experimental. The code follows the NCHL documents, but it is not tested with the NCHL system. NCHL gives credentials only to registered merchants.

If you have ConnectIPS test (UAT) credentials, you can help. Refer to [issue #3](https://github.com/VoidArchive/nepal-payment-go/issues/3).

## Installation

```bash
go get github.com/voidarchive/nepal-payment-go/v2
```

## Quick start

`Amount` is in paisa for each provider. One rupee is 100 paisa. Thus, Rs 100 is `10000`.

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

`GenerateQR` makes a QR code image. The customer scans the image to pay. `GenerateQR` needs an API username and an API password. Get them from your bank.

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

## How to upgrade from v1

Version 2 has these changes:

- The module path has `/v2` at the end. Change your imports.
- `Amount`, `ExpectedAmount` and `VerifiedAmount` are in paisa for each provider. Version 1 used rupees for eSewa and FonePay. Multiply those amounts by 100.
- ConnectIPS accepts only a PEM key. Remove `CertPassword` and `CertFormat` from your configuration.
- ConnectIPS `VerifyPayment` must have `ExpectedAmount`.
- eSewa, FonePay and ConnectIPS reject a callback if its order ID is different from `OriginalOrderID`.
- `core` does not contain the refund types, `ProviderInfo`, and some errors and fields that had no use.

## Payment verification

Use the same call to verify a payment for each provider:

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
    return
}

if result.Success {
    fmt.Printf("Payment complete! TxnID: %s", result.TransactionID)
}
```

Two providers have special rules:

- Khalti sends your order ID in the redirect, but the redirect has no signature, and the lookup reply does not contain the order ID. Thus, the SDK cannot compare it with `OriginalOrderID`. Keep the `TransactionID` from `InitiatePayment` (the Khalti `pidx`) with your order. Set `ProviderRef` to that value when you call `VerifyPayment`. The SDK rejects a callback that has a different `pidx`.
- ConnectIPS must have `ExpectedAmount`. If `ExpectedAmount` is 0, `VerifyPayment` gives an error.

## Payment records

The `payments` package keeps a record of each payment. It applies all the checks for you. Use it if you do not want to write this logic yourself.

```go
import (
    "github.com/voidarchive/nepal-payment-go/v2/core"
    "github.com/voidarchive/nepal-payment-go/v2/payments"
)

svc := &payments.Service{
    Store:     payments.NewMemoryStore(), // development only; use a database store in production
    Providers: map[string]core.Provider{"esewa": esewaProvider, "khalti": khaltiProvider},
    CallbackURL: func(id string) string {
        return "https://yoursite.com/payments/" + id + "/callback"
    },
}

// 1. Create the payment. Send the customer to p.Checkout.
p, err := svc.Create(ctx, payments.CreateParams{
    OrderID:  "ORDER-123",
    Provider: "esewa",
    Amount:   10000, // In paisa (NPR 100)
})

// 2. In your callback handler, give the query parameters to the service.
p, changed, err := svc.HandleCallback(ctx, id, callbackParams)
if changed && p.Status == payments.StatusPaid {
    // Complete the order. This block runs one time for each payment.
}

// 3. On a timer (for example, each minute), examine the payments that are not final.
changedPayments, err := svc.CheckDue(ctx)
```

The service obeys these rules:

- A payment has one of these statuses: `pending`, `paid`, `failed`, `expired`, `refunded`.
- If you call `Create` again with the same order ID, provider and amount, you get the same payment. A different amount gives `ErrAmountConflict`.
- If the order is paid, `Create` always gives the paid payment.
- `HandleCallback` always compares the amount, the payment ID and the provider reference. You cannot skip these checks.
- `changed` is true one time for each status change. This is true also when a callback and a status check occur at the same time.
- `CheckDue` asks the provider for the status of each pending payment. Thus, it finds a payment when the customer closes the browser before the redirect. The time between checks increases from 1 minute to 1 hour.
- A pending payment expires after 24 hours. FonePay cannot give the status without the redirect. Thus, a pending FonePay payment expires after 30 minutes.

Limits:

- `NewMemoryStore` loses all payments when the program stops. For production, write a `Store` for your database. The `payments/storetest` package contains tests that each `Store` must pass.
- A customer can start a payment with one provider and then pay with a different provider for the same order. If the customer pays the two payments, the two payments become `paid`. You must refund one of them.

### Mock provider

The `providers/mock` package is a provider for development. It does not need a merchant account. Its payment page has three buttons: "Pay", "Fail", and "Pay, then close the tab". Do not use it in production.

```go
import "github.com/voidarchive/nepal-payment-go/v2/providers/mock"

mockProvider, _ := mock.New(mock.Config{BaseURL: "http://localhost:8080/mockpay"})
http.Handle("/mockpay/", http.StripPrefix("/mockpay", mockProvider.Handler()))
```

## How to run the demo

The demo shows one checkout page for all four providers.

1. Start the demo:

   ```bash
   go run ./examples/demo/
   ```

2. Open http://localhost:8080 in a browser.

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

ConnectIPS support is experimental. Refer to [Test status](#test-status).

Register as a merchant with [Nepal Clearing House (NCHL)](https://nchl.com.np). NCHL gives you a Merchant ID, an App ID, an app password and a `.pfx` certificate file.

The SDK cannot read a `.pfx` file. It reads a PEM private key. Do this procedure one time:

1. Convert the `.pfx` file with OpenSSL. Type the PFX password when OpenSSL asks for it.

   ```bash
   openssl pkcs12 -in CREDITOR.pfx -nocerts -nodes -out creditor.pem
   ```

   If OpenSSL 3 shows an error about an algorithm that it does not support, add `-legacy` to the command. Old PFX files use RC2 encryption.

2. Set `CertPath` (or `CONNECTIPS_CERT_PATH`) to the path of `creditor.pem`.
3. Run `chmod 600 creditor.pem`. The PEM file has no password.
4. Do not add `creditor.pem` to git.

### FonePay

| Field | Sandbox Value |
|-------|---------------|
| Merchant Code | `NBQM` |
| Secret Key | `a7e3512f5032480a83137793cb2021dc` |

The FonePay sandbox is not always available. Get production credentials from FonePay.

## Environment variables

The demo reads these variables. The file `.env.example` contains the same list.

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
│   ├── fonepay/            # FonePay implementation
│   └── mock/               # Fake provider for development
├── payments/               # Payment records, status checks, Store interface
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

## Errors

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

## Tests

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

- Keep secret keys on the server. The customer can read all data that you send to the browser.
- Call `VerifyPayment` on the server before you complete an order. A redirect to your success URL is not proof of payment, because the customer controls the redirect.
- Always set `ExpectedAmount` and `OriginalOrderID` from your order record. If you do not, a customer can use the callback of a small order again for a large order.
- Use HTTPS callback URLs in production.
- The file `creditor.pem` has no password. Do not add it to git. Run `chmod 600` on it.

## Contributions

You can send a pull request. Before you send it, run `go test ./...` and `golangci-lint run ./...`.

## License

MIT License. Refer to [LICENSE](LICENSE).
