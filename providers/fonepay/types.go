package fonepay

// QRRequest contains the data needed to generate a dynamic QR code.
type QRRequest struct {
	// Amount in NPR (not paisa). Must be positive.
	Amount int64

	// OrderID is your unique transaction identifier (PRN).
	OrderID string

	// Description shown on the QR/payment screen (remarks1).
	Description string

	// Remarks2 for additional info (optional).
	Remarks2 string
}

// QRResponse contains the generated QR code data.
type QRResponse struct {
	// QRImageData contains the PNG image bytes of the QR code.
	QRImageData []byte

	// TransactionID for tracking this QR payment.
	TransactionID string

	// MerchantCode echoed back.
	MerchantCode string
}

// qrAPIRequest is the internal request payload for FonePay QR API.
type qrAPIRequest struct {
	Amount         string `json:"amount"`
	PRN            string `json:"prn"`
	MerchantCode   string `json:"merchantCode"`
	Remarks1       string `json:"remarks1"`
	Remarks2       string `json:"remarks2"`
	DataValidation string `json:"dataValidation"`
	Username       string `json:"username"`
	Password       string `json:"password"`
}

// qrAPIResponse is the internal response from FonePay QR API.
type qrAPIResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	QRCode       string `json:"qrCode"` // Base64 encoded image
	MerchantCode string `json:"merchantCode"`
	PRN          string `json:"prn"`
}
