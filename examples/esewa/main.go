package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/voidarchive/nepal-payment-go/core"
	"github.com/voidarchive/nepal-payment-go/providers/esewa"
)

var provider *esewa.Provider

func main() {
	secretKey := os.Getenv("ESEWA_SECRET_KEY")
	productCode := os.Getenv("ESEWA_PRODUCT_CODE")

	// Use sandbox defaults if not provided
	if secretKey == "" {
		secretKey = "8gBm/:&EnhH.1/q" // eSewa sandbox secret
	}
	if productCode == "" {
		productCode = "EPAYTEST" // eSewa sandbox product code
	}

	log.Printf("Using ProductCode: %s (set ESEWA_SECRET_KEY and ESEWA_PRODUCT_CODE for production)", productCode)

	var err error
	provider, err = esewa.New(esewa.Config{
		SecretKey:   secretKey,
		ProductCode: productCode,
		Sandbox:     productCode == "EPAYTEST", // Auto-detect sandbox mode
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/checkout", handleCheckout)
	http.HandleFunc("/payment/success", handleCallback)
	http.HandleFunc("/payment/failure", handleFailure)

	fmt.Print(`
eSewa Payment Demo Server Started!

Open: http://localhost:8080

Test credentials for eSewa sandbox:
- eSewa ID: 9806800001 (or 9806800002-9806800005)
- Password: Nepal@123
- MPIN: 1122 (for app)
- Token: 123456
`)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>eSewa Payment Demo</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; margin: 20px 0; }
        .btn { background: #60BB46; color: white; padding: 12px 24px; border: none; 
               border-radius: 4px; cursor: pointer; font-size: 16px; }
        .btn:hover { background: #4a9a35; }
        .price { font-size: 24px; font-weight: bold; color: #60BB46; }
    </style>
</head>
<body>
    <h1>Demo Store</h1>
    <div class="card">
        <h2>Premium Subscription</h2>
        <p>Access to all premium features for 1 month.</p>
        <p class="price">NPR 100.00</p>
        <form action="/checkout" method="POST" accept-charset="UTF-8">
            <button type="submit" class="btn">Pay with eSewa</button>
        </form>
    </div>
    <p style="color: #666; font-size: 14px;">
        This is a sandbox demo. Use test credentials shown in terminal.
    </p>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, tmpl)
}

func handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	orderID := fmt.Sprintf("ORDER-%d", os.Getpid())

	resp, err := provider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      100, // NPR 100
		Currency:    "NPR",
		OrderID:     orderID,
		Description: "Premium Subscription",
		CallbackURL: "http://localhost:8080/payment/success",
	})
	if err != nil {
		log.Printf("Payment initiation failed: %v", err)
		http.Error(w, fmt.Sprintf("Payment initiation failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Payment initiated: order=%s, redirecting to eSewa...", orderID)

	// Generate auto-submitting form for eSewa
	renderFormPOST(w, resp.TargetURL, resp.FormFields)
}

func renderFormPOST(w http.ResponseWriter, targetURL string, fields map[string]string) {
	tmpl := template.Must(template.New("form").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Redirecting to eSewa...</title>
    <style>
        body { font-family: system-ui; display: flex; justify-content: center; 
               align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
        .loader { text-align: center; }
        .spinner { border: 4px solid #f3f3f3; border-top: 4px solid #60BB46; 
                   border-radius: 50%; width: 40px; height: 40px; 
                   animation: spin 1s linear infinite; margin: 0 auto 20px; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="loader">
        <div class="spinner"></div>
        <p>Redirecting to eSewa...</p>
    </div>
    <form id="esewa-form" action="{{.URL}}" method="POST" style="display: none;">
        {{range $key, $value := .Fields}}
        <input type="hidden" name="{{$key}}" value="{{$value}}">
        {{end}}
    </form>
    <script>document.getElementById('esewa-form').submit();</script>
</body>
</html>`))

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, map[string]any{
		"URL":    targetURL,
		"Fields": fields,
	})
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	log.Printf("Success callback received: %v", r.URL.Query())

	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		params[key] = values[0]
	}

	result, err := provider.VerifyPayment(r.Context(), &core.VerifyRequest{
		EncodedParams:  params,
		ExpectedAmount: 100,
	})
	if err != nil {
		log.Printf("Verification error: %v", err)
		msg := "Payment verification failed"
		if errors.Is(err, core.ErrAmountMismatch) {
			msg = "Amount mismatch detected!"
		} else if errors.Is(err, core.ErrSignatureMismatch) {
			msg = "Signature verification failed!"
		}
		renderResult(w, false, "Verification Failed", msg, params)
		return
	}

	log.Printf("Payment verified: success=%v, txn=%s, amount=%d, status=%s",
		result.Success, result.TransactionID, result.VerifiedAmount, result.Status)

	if result.Success {
		renderResult(w, true, "Payment Successful!",
			fmt.Sprintf("Transaction ID: %s<br>Amount: NPR %.2f",
				result.TransactionID, float64(result.VerifiedAmount)), params)
	} else {
		renderResult(w, false, "Payment Not Completed",
			fmt.Sprintf("Status: %s", result.Status), params)
	}
}

func handleFailure(w http.ResponseWriter, r *http.Request) {
	log.Printf("Failure callback received: %v", r.URL.Query())

	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		params[key] = values[0]
	}

	renderResult(w, false, "Payment Failed", "The payment was not completed.", params)
}

func renderResult(w http.ResponseWriter, success bool, title, message string, params map[string]string) {
	color := "#dc3545"
	if success {
		color = "#28a745"
	}

	tmpl := template.Must(template.New("result").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
        .card { border: 2px solid {{.Color}}; border-radius: 8px; padding: 20px; margin: 20px 0; }
        h1 { color: {{.Color}}; }
        .params { background: #f5f5f5; padding: 10px; border-radius: 4px; font-size: 12px; }
        a { color: #60BB46; }
    </style>
</head>
<body>
    <div class="card">
        <h1>{{.Title}}</h1>
        <p>{{.Message}}</p>
        <p><a href="/">Back to Store</a></p>
    </div>
    <details>
        <summary>Debug: Callback Parameters</summary>
        <div class="params"><pre>{{.Params}}</pre></div>
    </details>
</body>
</html>`))

	var sb strings.Builder
	for k, v := range params {
		fmt.Fprintf(&sb, "%s: %s\n", k, v)
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, map[string]any{
		"Title":   title,
		"Message": template.HTML(message),
		"Color":   color,
		"Params":  sb.String(),
	})
}
