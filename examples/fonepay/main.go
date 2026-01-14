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
	"github.com/voidarchive/nepal-payment-go/providers/fonepay"
)

var provider *fonepay.Provider

func main() {
	merchantCode := os.Getenv("FONEPAY_MERCHANT_CODE")
	secretKey := os.Getenv("FONEPAY_SECRET_KEY")

	// Use sandbox defaults if not provided
	if merchantCode == "" {
		merchantCode = "NBQM" // FonePay sandbox merchant code
	}
	if secretKey == "" {
		secretKey = "a7e3512f5032480a83137793cb2021dc" // FonePay sandbox secret
	}

	log.Printf("Using MerchantCode: %s (set FONEPAY_MERCHANT_CODE and FONEPAY_SECRET_KEY for production)", merchantCode)

	var err error
	provider, err = fonepay.New(fonepay.Config{
		MerchantCode: merchantCode,
		SecretKey:    secretKey,
		Sandbox:      merchantCode == "NBQM", // Auto-detect sandbox mode
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/checkout", handleCheckout)
	http.HandleFunc("/payment/callback", handleCallback)

	fmt.Print(`
FonePay Payment Demo Server Started!

Open: http://localhost:8080

Sandbox credentials (auto-configured):
- Merchant Code: NBQM
- Secret Key: a7e3512f5032480a83137793cb2021dc

Set FONEPAY_MERCHANT_CODE and FONEPAY_SECRET_KEY for production.
`)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>FonePay Payment Demo</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; margin: 20px 0; }
        .btn { background: #e31837; color: white; padding: 12px 24px; border: none; 
               border-radius: 4px; cursor: pointer; font-size: 16px; }
        .btn:hover { background: #c01530; }
        .price { font-size: 24px; font-weight: bold; color: #e31837; }
    </style>
</head>
<body>
    <h1>Demo Store</h1>
    <div class="card">
        <h2>Premium Subscription</h2>
        <p>Access to all premium features for 1 month.</p>
        <p class="price">NPR 100.00</p>
        <form action="/checkout" method="POST" accept-charset="UTF-8">
            <button type="submit" class="btn">Pay with FonePay</button>
        </form>
    </div>
    <p style="color: #666; font-size: 14px;">
        This is a sandbox demo. Using default test credentials.
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
		CallbackURL: "http://localhost:8080/payment/callback",
	})
	if err != nil {
		log.Printf("Payment initiation failed: %v", err)
		http.Error(w, fmt.Sprintf("Payment initiation failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Payment initiated: order=%s, redirecting to FonePay...", orderID)

	// FonePay uses redirect, so redirect the user
	http.Redirect(w, r, resp.TargetURL, http.StatusFound)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	log.Printf("Callback received: %v", r.URL.Query())

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
			fmt.Sprintf("Transaction ID: %s<br>Order: %s",
				result.TransactionID, result.OrderID), params)
	} else {
		renderResult(w, false, "Payment Not Completed",
			fmt.Sprintf("Status: %s", result.Status), params)
	}
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
        a { color: #e31837; }
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
