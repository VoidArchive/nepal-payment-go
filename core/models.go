// Package core defines the contracts for Nepal payment provider implementations.
// It is intentionally dependency-free, using only the standard library.
package core

// PaymentRequest contains the data needed to initiate a payment.
type PaymentRequest struct {
	// Amount in smallest currency unit (e.g., Paisa). Must be positive.
	Amount int64

	// Currency code (ISO 4217, e.g., "NPR").
	Currency string

	// OrderID is your unique transaction identifier.
	OrderID string

	// Description shown to customer during checkout.
	Description string

	// CallbackURL where provider redirects after payment.
	CallbackURL string

	// CustomerEmail for receipts and fraud detection (optional).
	CustomerEmail string

	// CustomerPhone for authentication (optional).
	CustomerPhone string

	// Metadata for provider-specific or custom data.
	Metadata map[string]string
}

// PaymentResponse contains the result of initiating a payment.
//
// Check IsFormPOST to determine the integration flow:
//   - false: Redirect user directly to TargetURL
//   - true: Generate HTML form with FormFields and POST to TargetURL
type PaymentResponse struct {
	// IsFormPOST indicates whether to use form POST (eSewa) vs redirect (Khalti).
	IsFormPOST bool

	// TargetURL is the payment gateway endpoint.
	TargetURL string

	// FormFields contains hidden form inputs (only when IsFormPOST is true).
	FormFields map[string]string

	// TransactionID from the provider (may be empty at initiation).
	TransactionID string

	// ProviderReferenceID for reconciliation (optional).
	ProviderReferenceID string
}

// VerifyRequest contains callback data for payment verification.
type VerifyRequest struct {
	// EncodedParams contains raw callback parameters from the provider.
	EncodedParams map[string]string

	// OriginalOrderID to match against (optional).
	OriginalOrderID string

	// ExpectedAmount to validate against (0 skips validation).
	ExpectedAmount int64

	// ExpectedCurrency to validate against.
	ExpectedCurrency string
}

// VerifyResponse contains the verified payment details.
type VerifyResponse struct {
	// Success indicates payment was verified and completed.
	Success bool

	// TransactionID confirmed by the provider.
	TransactionID string

	// OrderID echoed from your original request.
	OrderID string

	// VerifiedAmount confirmed by the provider (in smallest unit).
	VerifiedAmount int64

	// Currency of the payment.
	Currency string

	// Status normalized to: COMPLETE, PENDING, FAILED, REFUNDED, CANCELLED.
	Status string

	// ProviderResponse contains raw response for debugging.
	ProviderResponse map[string]string

	// VerifiedAt timestamp (Unix seconds).
	VerifiedAt int64
}
