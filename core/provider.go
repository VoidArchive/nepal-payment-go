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
