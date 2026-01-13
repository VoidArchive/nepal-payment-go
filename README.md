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
| eSewa | ✅ Implemented | Form POST |
| ConnectIPS | Planned | Redirect |

## Installation

```bash
go get github.com/voidarchive/nepal-payment-go
```

## Quick Start

### Khalti

```go
import "github.com/voidarchive/nepal-payment-go/providers/khalti"

provider, _ := khalti.New(khalti.Config{
    SecretKey:  os.Getenv("KHALTI_SECRET_KEY"),
    WebsiteURL: "https://yoursite.com",
    Sandbox:    true,
})

resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      10000, // NPR 100.00 in paisa
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})
// Redirect user to resp.TargetURL
```

### eSewa

```go
import "github.com/voidarchive/nepal-payment-go/providers/esewa"

provider, _ := esewa.New(esewa.Config{
    SecretKey:   os.Getenv("ESEWA_SECRET_KEY"),
    ProductCode: os.Getenv("ESEWA_PRODUCT_CODE"),
    Sandbox:     true,
})

resp, _ := provider.InitiatePayment(ctx, &core.PaymentRequest{
    Amount:      100, // NPR 100
    OrderID:     "ORDER-123",
    CallbackURL: "https://yoursite.com/callback",
})
// resp.IsFormPOST = true, generate HTML form with resp.FormFields
```

## Running the Examples

### Khalti Demo

```bash
export KHALTI_SECRET_KEY="your_key"  # Get from test-admin.khalti.com
go run ./examples/khalti/
```

### eSewa Demo

```bash
go run ./examples/esewa/  # Uses sandbox defaults automatically
```

## Sandbox Test Credentials

### Khalti
| Field | Value |
|-------|-------|
| Khalti ID | 9800000001 - 9800000005 |
| MPIN | 1111 |
| OTP | 987654 |

### eSewa
| Field | Value |
|-------|-------|
| eSewa ID | 9806800001 - 9806800005 |
| Password | Nepal@123 |
| Token | 123456 |

## Project Structure

```
.
├── core/                    # Core contracts (dependency-free)
│   ├── models.go           # Request/Response types
│   ├── errors.go           # Sentinel errors
│   ├── provider.go         # Provider interface
│   └── mock.go             # Mock provider for testing
├── providers/
│   ├── khalti/             # Khalti implementation
│   └── esewa/              # eSewa implementation
└── examples/
    ├── khalti/             # Khalti demo server
    └── esewa/              # eSewa demo server
```

## Testing

```bash
go test ./... -v
```

## License

MIT License

