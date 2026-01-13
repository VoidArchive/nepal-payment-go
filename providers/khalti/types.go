package khalti

// Khalti API request/response types.
// These are internal to the provider and not exported.

// initiateRequest is the payload for /epayment/initiate/
type initiateRequest struct {
	ReturnURL         string        `json:"return_url"`
	WebsiteURL        string        `json:"website_url"`
	Amount            int64         `json:"amount"`
	PurchaseOrderID   string        `json:"purchase_order_id"`
	PurchaseOrderName string        `json:"purchase_order_name"`
	CustomerInfo      *customerInfo `json:"customer_info,omitempty"`
}

type customerInfo struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// initiateResponse is the response from /epayment/initiate/
type initiateResponse struct {
	PIDX       string `json:"pidx"`
	PaymentURL string `json:"payment_url"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	ExpiresIn  int    `json:"expires_in,omitempty"`
}

// lookupRequest is the payload for /epayment/lookup/
type lookupRequest struct {
	PIDX string `json:"pidx"`
}

// lookupResponse is the response from /epayment/lookup/
type lookupResponse struct {
	PIDX          string `json:"pidx"`
	TotalAmount   int64  `json:"total_amount"`
	Status        string `json:"status"`
	TransactionID string `json:"transaction_id"`
	Fee           int64  `json:"fee"`
	Refunded      bool   `json:"refunded"`
}

// errorResponse is the error format from Khalti API.
type errorResponse struct {
	Detail     string            `json:"detail"`
	ErrorKey   string            `json:"error_key,omitempty"`
	Errors     map[string]string `json:"errors,omitempty"`
	StatusCode int               `json:"status_code,omitempty"`
}
