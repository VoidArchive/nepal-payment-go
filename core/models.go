// Package core defines the contracts for Nepal payment provider implementations.
// It is intentionally dependency-free, using only the standard library.
package core

// PaymentRequest contains the data needed to initiate a payment.
type PaymentRequest struct {
	// Amount in paisa (1/100 rupee) for every provider. Must be positive.
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
	// For Khalti this is the pidx. Store it and pass it as VerifyRequest.ProviderRef.
	TransactionID string
}

// VerifyRequest contains callback data for payment verification.
type VerifyRequest struct {
	// EncodedParams contains raw callback parameters from the provider.
	EncodedParams map[string]string

	// OriginalOrderID: when set, a callback with a different order ID is rejected.
	// Always set it. Not checked for Khalti (its lookup has no order ID): set ProviderRef instead.
	OriginalOrderID string

	// ProviderRef is the PaymentResponse.TransactionID returned by InitiatePayment for this order.
	// When set, a callback that carries a different provider reference is rejected.
	ProviderRef string

	// ExpectedAmount in paisa. 0 skips the check, except connectIPS where it is required.
	ExpectedAmount int64
}

// VerifyResponse contains the verified payment details.
type VerifyResponse struct {
	// Success indicates payment was verified and completed.
	Success bool

	// TransactionID confirmed by the provider.
	TransactionID string

	// OrderID echoed from your original request.
	OrderID string

	// VerifiedAmount confirmed by the provider, in paisa.
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

// StatusRequest identifies a payment for a status check without callback data.
type StatusRequest struct {
	OrderID     string // the order ID sent to the gateway in InitiatePayment
	ProviderRef string // PaymentResponse.TransactionID from InitiatePayment (Khalti: pidx)
	Amount      int64  // paisa; the amount of the payment
}
