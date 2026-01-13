package core

import "context"

// Provider defines the contract for payment gateway implementations.
// All methods must be safe for concurrent use.
type Provider interface {
	// InitiatePayment creates a payment session with the provider.
	// Returns ErrInvalidRequest for validation failures, ErrProviderError for API failures.
	InitiatePayment(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error)

	// VerifyPayment validates a callback and confirms payment status.
	// Returns ErrSignatureMismatch for tampering, ErrAmountMismatch for manipulation.
	VerifyPayment(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error)
}

// ProviderInfo contains metadata about a payment provider.
type ProviderInfo struct {
	Name                string
	Code                string
	LogoURL             string
	SupportedCurrencies []string
	MinAmount           int64
	MaxAmount           int64 // 0 means no maximum
	IsTestMode          bool
}

// RefundRequest contains data for refunding a payment.
type RefundRequest struct {
	TransactionID string
	Amount        int64
	Reason        string
	Metadata      map[string]string
}

// RefundResponse contains the result of a refund.
type RefundResponse struct {
	Success               bool
	RefundID              string
	OriginalTransactionID string
	RefundedAmount        int64
	Status                string
}

// RefundProvider extends Provider with refund capability.
type RefundProvider interface {
	Provider
	RefundPayment(ctx context.Context, req *RefundRequest) (*RefundResponse, error)
}
