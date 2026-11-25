package connectips

import "encoding/json"

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
// merchantId (a number) and token (null) are left out: nothing reads them.
type validationResponse struct {
	Status      string      `json:"status"`
	StatusDesc  string      `json:"statusDesc"`
	ReferenceID string      `json:"referenceId"`
	TxnAmt      json.Number `json:"txnAmt"` // a string ("500") or a number (1000.00)
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
