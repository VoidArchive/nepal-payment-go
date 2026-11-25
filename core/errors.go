package core

import "errors"

// Sentinel errors for consistent error handling across providers.
// Use errors.Is() to check; providers should wrap these with context.
var (
	ErrPaymentNotFound   = errors.New("payment: transaction not found")
	ErrSignatureMismatch = errors.New("payment: signature verification failed")
	ErrAmountMismatch    = errors.New("payment: amount mismatch")
	ErrProviderError     = errors.New("payment: provider error")
	ErrInvalidConfig     = errors.New("payment: invalid provider configuration")
	ErrInvalidRequest    = errors.New("payment: invalid request")
	ErrRateLimited       = errors.New("payment: rate limit exceeded")
	ErrTimeout           = errors.New("payment: operation timed out")
)

// Normalized payment status values.
const (
	StatusComplete  = "COMPLETE"
	StatusPending   = "PENDING"
	StatusFailed    = "FAILED"
	StatusRefunded  = "REFUNDED"
	StatusCancelled = "CANCELLED"
	StatusExpired   = "EXPIRED"
)
