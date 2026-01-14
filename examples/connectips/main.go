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
	"github.com/voidarchive/nepal-payment-go/providers/connectips"
)

var provider *connectips.Provider

func main() {
	merchantID := os.Getenv("CONNECTIPS_MERCHANT_ID")
	appID := os.Getenv("CONNECTIPS_APP_ID")
	appName := os.Getenv("CONNECTIPS_APP_NAME")
	certPath := os.Getenv("CONNECTIPS_CERT_PATH")
	certPassword := os.Getenv("CONNECTIPS_CERT_PASSWORD")

	if merchantID == "" || appID == "" || certPath == "" {
		log.Fatal(`Missing required environment variables.

Set the following environment variables:
  CONNECTIPS_MERCHANT_ID   - Creditor merchant ID from ConnectIPS
  CONNECTIPS_APP_ID        - Application ID from ConnectIPS
  CONNECTIPS_APP_NAME      - Application name (optional)
  CONNECTIPS_CERT_PATH     - Path to .pfx or .pem certificate file
  CONNECTIPS_CERT_PASSWORD - Password for .pfx file

Get sandbox credentials by onboarding with Nepal Clearing House (NCHL).
`)
	}

	if appName == "" {
		appName = "Nepal Payment Demo"
	}

	var err error
	provider, err = connectips.New(connectips.Config{
		MerchantID:   merchantID,
		AppID:        appID,
		AppName:      appName,
		CertPath:     certPath,
		CertPassword: certPassword,
		Sandbox:      true, // Use sandbox by default
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/checkout", handleCheckout)
	http.HandleFunc("/payment/success", handleCallback)
	http.HandleFunc("/payment/failure", handleFailure)

	fmt.Print(`
ConnectIPS Payment Demo Server Started!

Open: http://localhost:8080

Note: This demo requires valid ConnectIPS sandbox credentials.
Get credentials by onboarding with Nepal Clearing House (NCHL).

Environment variables required:
  CONNECTIPS_MERCHANT_ID
  CONNECTIPS_APP_ID
  CONNECTIPS_CERT_PATH
  CONNECTIPS_CERT_PASSWORD
`)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>ConnectIPS Payment Demo</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; margin: 20px 0; }
        .btn { background: #1a237e; color: white; padding: 12px 24px; border: none; 
               border-radius: 4px; cursor: pointer; font-size: 16px; }
        .btn:hover { background: #0d47a1; }
        .price { font-size: 24px; font-weight: bold; color: #1a237e; }
        .info { background: #e3f2fd; padding: 10px; border-radius: 4px; margin-top: 20px; font-size: 14px; }
    </style>
</head>
<body>
    <h1>Demo Store</h1>
    <div class="card">
        <h2>Premium Subscription</h2>
        <p>Access to all premium features for 1 month.</p>
        <p class="price">NPR 100.00</p>
        <form action="/checkout" method="POST" accept-charset="UTF-8">
            <button type="submit" class="btn">Pay with ConnectIPS</button>
        </form>
    </div>
    <div class="info">
        <strong>About ConnectIPS:</strong><br>
        ConnectIPS is a real-time payment system by Nepal Clearing House Limited (NCHL).
        It enables direct bank-to-bank payments using digital signatures for security.
    </div>
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

	log.Printf("Payment initiated: order=%s, redirecting to ConnectIPS...", orderID)

	// Generate auto-submitting form
	renderFormPOST(w, resp.TargetURL, resp.FormFields)
}

func renderFormPOST(w http.ResponseWriter, targetURL string, fields map[string]string) {
	tmpl := template.Must(template.New("form").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Redirecting to ConnectIPS...</title>
    <style>
        body { font-family: system-ui; display: flex; justify-content: center; 
               align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
        .loader { text-align: center; }
        .spinner { border: 4px solid #f3f3f3; border-top: 4px solid #1a237e; 
                   border-radius: 50%; width: 40px; height: 40px; 
                   animation: spin 1s linear infinite; margin: 0 auto 20px; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="loader">
        <div class="spinner"></div>
        <p>Redirecting to ConnectIPS...</p>
    </div>
    <form id="connectips-form" action="{{.URL}}" method="POST" style="display: none;">
        {{range $key, $value := .Fields}}
        <input type="hidden" name="{{$key}}" value="{{$value}}">
        {{end}}
    </form>
    <script>document.getElementById('connectips-form').submit();</script>
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
		} else if errors.Is(err, core.ErrPaymentNotFound) {
			msg = "Transaction not found!"
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
        a { color: #1a237e; }
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
