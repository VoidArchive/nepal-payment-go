package esewa

// eSewa API request/response types.
// These are internal to the provider and not exported.

// callbackResponse is the decoded JSON from Base64 callback parameter.
type callbackResponse struct {
	TransactionCode  string `json:"transaction_code"`
	Status           string `json:"status"`
	TotalAmount      string `json:"total_amount"` // eSewa returns as string
	TransactionUUID  string `json:"transaction_uuid"`
	ProductCode      string `json:"product_code"`
	SignedFieldNames string `json:"signed_field_names"`
	Signature        string `json:"signature"`
}

// statusResponse is returned by the transaction status check API.
type statusResponse struct {
	ProductCode     string  `json:"product_code"`
	TransactionUUID string  `json:"transaction_uuid"`
	TotalAmount     float64 `json:"total_amount"`
	Status          string  `json:"status"`
	RefID           *string `json:"ref_id"`
}

// errorResponse is returned when the status check API fails.
type errorResponse struct {
	Code         int    `json:"code"`
	ErrorMessage string `json:"error_message"`
}
