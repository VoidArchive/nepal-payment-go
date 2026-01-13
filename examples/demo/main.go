package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/voidarchive/nepal-payment-go/core"
	"github.com/voidarchive/nepal-payment-go/providers/esewa"
	"github.com/voidarchive/nepal-payment-go/providers/khalti"
)

var (
	esewaProvider  *esewa.Provider
	khaltiProvider *khalti.Provider
)

func main() {
	initEsewa()
	initKhalti()

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/checkout/esewa", handleEsewaCheckout)
	http.HandleFunc("/checkout/khalti", handleKhaltiCheckout)
	http.HandleFunc("/payment/esewa/success", handleEsewaCallback)
	http.HandleFunc("/payment/esewa/failure", handleEsewaFailure)
	http.HandleFunc("/payment/khalti/callback", handleKhaltiCallback)

	fmt.Print(`
Nepal Payment Demo Server Started!

Open: http://localhost:8080

Supported Providers:
- eSewa (Sandbox)
- Khalti (Sandbox)
`)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func initEsewa() {
	secretKey := os.Getenv("ESEWA_SECRET_KEY")
	productCode := os.Getenv("ESEWA_PRODUCT_CODE")

	if secretKey == "" {
		secretKey = "8gBm/:&EnhH.1/q"
	}
	if productCode == "" {
		productCode = "EPAYTEST"
	}

	var err error
	esewaProvider, err = esewa.New(esewa.Config{
		SecretKey:   secretKey,
		ProductCode: productCode,
		Sandbox:     productCode == "EPAYTEST",
	})
	if err != nil {
		log.Fatalf("Failed to create eSewa provider: %v", err)
	}
}

func initKhalti() {
	secretKey := os.Getenv("KHALTI_SECRET_KEY")
	if secretKey == "" {
		log.Println("WARNING: KHALTI_SECRET_KEY not set. Khalti payments will fail.")
		return
	}

	var err error
	khaltiProvider, err = khalti.New(khalti.Config{
		SecretKey:  secretKey,
		WebsiteURL: "http://localhost:8080",
		Sandbox:    true,
	})
	if err != nil {
		log.Fatalf("Failed to create Khalti provider: %v", err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("examples", "demo", "checkout.html"))
}

func handleEsewaCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	orderID := fmt.Sprintf("ESEWA-%d", os.Getpid())

	resp, err := esewaProvider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      100,
		Currency:    "NPR",
		OrderID:     orderID,
		Description: "Premium Subscription",
		CallbackURL: "http://localhost:8080/payment/esewa/success",
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("eSewa initiation failed: %v", err), http.StatusInternalServerError)
		return
	}

	renderFormPOST(w, resp.TargetURL, resp.FormFields)
}

func handleKhaltiCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if khaltiProvider == nil {
		http.Error(w, "Khalti provider not configured", http.StatusInternalServerError)
		return
	}

	// Khalti amount is in paisa
	resp, err := khaltiProvider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      10000,
		Currency:    "NPR",
		OrderID:     fmt.Sprintf("KHALTI-%d", os.Getpid()),
		Description: "Premium Subscription",
		CallbackURL: "http://localhost:8080/payment/khalti/callback",
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Khalti initiation failed: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, resp.TargetURL, http.StatusFound)
}

func handleEsewaCallback(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r, esewaProvider, 100, "eSewa")
}

func handleEsewaFailure(w http.ResponseWriter, r *http.Request) {
	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		params[key] = values[0]
	}
	renderResult(w, false, "Payment Failed", "The payment was not completed.", params)
}

func handleKhaltiCallback(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r, khaltiProvider, 10000, "Khalti")
}

func handleCallback(w http.ResponseWriter, r *http.Request, p core.Provider, amount int64, name string) {
	log.Printf("%s callback received: %v", name, r.URL.Query())

	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		params[key] = values[0]
	}

	// Check for Khalti cancellation
	if name == "Khalti" && params["status"] == "User canceled" {
		renderResult(w, false, "Payment Cancelled", "You cancelled the payment.", params)
		return
	}

	result, err := p.VerifyPayment(r.Context(), &core.VerifyRequest{
		EncodedParams:  params,
		ExpectedAmount: amount,
	})
	if err != nil {
		log.Printf("Verification error: %v", err)
		msg := "Payment verification failed"
		if errors.Is(err, core.ErrAmountMismatch) {
			msg = "Amount mismatch detected!"
		}
		renderResult(w, false, "Verification Failed", msg, params)
		return
	}

	if result.Success {
		displayAmount := float64(result.VerifiedAmount)
		if name == "Khalti" {
			displayAmount = displayAmount / 100
		}
		renderResult(w, true, "Payment Successful!",
			fmt.Sprintf("Provider: %s<br>Transaction ID: %s<br>Amount: NPR %.2f",
				name, result.TransactionID, displayAmount), params)
	} else {
		renderResult(w, false, "Payment Not Completed",
			fmt.Sprintf("Status: %s", result.Status), params)
	}
}

func renderFormPOST(w http.ResponseWriter, targetURL string, fields map[string]string) {
	tmpl := template.Must(template.New("form").Parse(`<!DOCTYPE html>
<html>
<head>
    <title>Redirecting...</title>
</head>
<body>
    <p>Redirecting to payment provider...</p>
    <form id="pay-form" action="{{.URL}}" method="POST">
        {{range $key, $value := .Fields}}
        <input type="hidden" name="{{$key}}" value="{{$value}}">
        {{end}}
    </form>
    <script>document.getElementById('pay-form').submit();</script>
</body>
</html>`))

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, map[string]any{
		"URL":    targetURL,
		"Fields": fields,
	})
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
        a { color: #007bff; text-decoration: none; }
        a:hover { text-decoration: underline; }
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
