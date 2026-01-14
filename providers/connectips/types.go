package connectips

// ConnectIPS API request/response types.
// These are internal to the provider and not exported.

// validationRequest is the payload for /api/creditor/validatetxn
type validationRequest struct {
	MerchantID  string `json:"merchantId"`
	AppID       string `json:"appId"`
	ReferenceID string `json:"referenceId"`
	TxnAmount   int64  `json:"txnAmt"`
	Token       string `json:"token"`
}

// validationResponse is returned by the validation API.
type validationResponse struct {
	Status      string `json:"status"`
	StatusDesc  string `json:"statusDesc,omitempty"`
	TransRefID  string `json:"transactionRefId,omitempty"`
	ReferenceID string `json:"referenceId,omitempty"`
	TxnAmount   int64  `json:"txnAmount,omitempty"`
	TxnDate     string `json:"txnDate,omitempty"`
	TokenValid  bool   `json:"tokenValid,omitempty"`
}

// ConnectIPS status codes.
const (
	statusSuccess        = "SUCCESS"
	statusPending        = "PENDING"
	statusCancelled      = "CANCELLED"
	statusTimeout        = "TIMEOUT"
	statusInvalidToken   = "INVALID_TOKEN"
	statusNotFound       = "NOT_FOUND"
	statusSessionExpired = "SESSION_EXPIRED"
)
