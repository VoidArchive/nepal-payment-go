# Nepal Payment Go

A unified payment SDK for Nepal, providing a clean interface for multiple payment providers.

## Features

- **Zero external dependencies** - Only uses Go standard library
- **Provider-agnostic** - Single interface for all payment providers
- **Type-safe** - Strongly typed request/response models
- **Context-aware** - All methods accept `context.Context` for cancellation and timeouts
- **Money handling** - Uses `int64` for amounts in smallest currency unit (Paisa)
- **Hybrid flow support** - Supports both redirect and form POST payment flows

## Supported Providers

| Provider | Status | Flow Type |
|----------|--------|-----------|
| Khalti | ✅ Implemented | Redirect |
| eSewa | 🚧 Planned | Form POST |
| ConnectIPS | 🚧 Planned | Redirect |

## Installation

```bash
go get github.com/voidarchive/nepal-payment-go
```

## Quick Start

### Khalti Integration

```go
package main

import (
    "context"
    "log"
    
    "github.com/voidarchive/nepal-payment-go/core"
    "github.com/voidarchive/nepal-payment-go/providers/khalti"
)

func main() {
    // Initialize provider
    provider, err := khalti.New(khalti.Config{
        SecretKey:  "your_live_secret_key",
        WebsiteURL: "https://yoursite.com",
        Sandbox:    true, // false for production
    })
    if err != nil {
        log.Fatal(err)
    }

    // Initiate payment
    resp, err := provider.InitiatePayment(context.Background(), &core.PaymentRequest{
        Amount:      10000, // NPR 100.00 in Paisa
        Currency:    "NPR",
        OrderID:     "ORDER-123",
        Description: "Premium Plan",
        CallbackURL: "https://yoursite.com/payment/callback",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Redirect user to resp.TargetURL
    log.Printf("Redirect to: %s", resp.TargetURL)
}
```

### Verifying Payment Callback

```go
func handleCallback(w http.ResponseWriter, r *http.Request) {
    // Collect callback parameters
    params := make(map[string]string)
    for key, values := range r.URL.Query() {
        params[key] = values[0]
    }

    // Verify payment
    result, err := provider.VerifyPayment(r.Context(), &core.VerifyRequest{
        EncodedParams:  params,
        ExpectedAmount: 10000,
    })
    if err != nil {
        // Handle error
        if errors.Is(err, core.ErrAmountMismatch) {
            log.Println("Security: Amount mismatch!")
        }
        return
    }

    if result.Success {
        log.Printf("Payment successful: %s", result.TransactionID)
        // Fulfill order
    }
}
```

## Project Structure

```
.
├── core/                    # Core contracts (dependency-free)
│   ├── models.go           # Request/Response types
│   ├── errors.go           # Sentinel errors
│   ├── provider.go         # Provider interface
│   └── mock.go             # Mock provider for testing
├── providers/
│   └── khalti/             # Khalti implementation
│       ├── khalti.go       # Provider implementation
│       ├── types.go        # Khalti-specific types
│       └── khalti_test.go  # Tests
└── examples/
    └── khalti/             # Working example
        └── main.go
```

## Core Package

The `core` package defines the contracts that all providers implement:

### PaymentRequest

```go
type PaymentRequest struct {
    Amount        int64             // Amount in smallest unit (Paisa)
    Currency      string            // ISO 4217 (e.g., "NPR")
    OrderID       string            // Your unique order identifier
    Description   string            // Payment description
    CallbackURL   string            // Redirect URL after payment
    CustomerEmail string            // Optional
    CustomerPhone string            // Optional
    Metadata      map[string]string // Provider-specific data
}
```

### PaymentResponse

```go
type PaymentResponse struct {
    IsFormPOST    bool              // true = form POST, false = redirect
    TargetURL     string            // Payment gateway URL
    FormFields    map[string]string // Form fields (when IsFormPOST=true)
    TransactionID string            // Provider's transaction ID
}
```

### Errors

```go
core.ErrPaymentNotFound      // Transaction doesn't exist
core.ErrSignatureMismatch    // Callback tampering detected
core.ErrAmountMismatch       // Amount manipulation detected
core.ErrProviderError        // Provider API error
core.ErrInvalidConfig        // Invalid configuration
core.ErrInvalidRequest       // Invalid request parameters
```

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific provider tests
go test ./providers/khalti/... -v
```

## Running the Example

```bash
# Set your Khalti sandbox key
export KHALTI_SECRET_KEY="your_live_secret_key"

# Run the example server
go run ./examples/khalti/

# Open http://localhost:8080
```

### Khalti Sandbox Credentials

| Field | Value |
|-------|-------|
| Khalti ID | 9800000001 - 9800000005 |
| MPIN | 1111 |
| OTP | 987654 |

## License

MIT License - see [LICENSE](LICENSE) for details.
