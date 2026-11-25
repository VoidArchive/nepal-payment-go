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
	"time"

	"github.com/voidarchive/nepal-payment-go/v2/core"
	"github.com/voidarchive/nepal-payment-go/v2/providers/connectips"
	"github.com/voidarchive/nepal-payment-go/v2/providers/esewa"
	"github.com/voidarchive/nepal-payment-go/v2/providers/fonepay"
	"github.com/voidarchive/nepal-payment-go/v2/providers/khalti"
)

var (
	esewaProvider      *esewa.Provider
	khaltiProvider     *khalti.Provider
	connectipsProvider *connectips.Provider
	fonepayProvider    *fonepay.Provider
)

func main() {
	initProviders()

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/checkout/esewa", handleEsewaCheckout)
	http.HandleFunc("/checkout/khalti", handleKhaltiCheckout)
	http.HandleFunc("/checkout/connectips", handleConnectIPSCheckout)
	http.HandleFunc("/checkout/fonepay", handleFonepayCheckout)
	http.HandleFunc("/payment/esewa/success", handleEsewaCallback)
	http.HandleFunc("/payment/esewa/failure", handleEsewaFailure)
	http.HandleFunc("/payment/khalti/callback", handleKhaltiCallback)
	http.HandleFunc("/payment/connectips/callback", handleConnectIPSCallback)
	http.HandleFunc("/payment/fonepay/callback", handleFonepayCallback)

	fmt.Print(`
╔══════════════════════════════════════════════════════════════════╗
║           Nepal Payment SDK - Combined Demo                      ║
╠══════════════════════════════════════════════════════════════════╣
║  Open: http://localhost:8080                                     ║
╠══════════════════════════════════════════════════════════════════╣
║  Providers:                                                      ║
║    ✓ eSewa      - Sandbox (auto-configured)                      ║
║    ✓ Khalti     - Needs KHALTI_SECRET_KEY                        ║
║    ✓ ConnectIPS - Needs NCHL credentials + certificate           ║
║    ✓ FonePay    - Sandbox (auto-configured)                      ║
╚══════════════════════════════════════════════════════════════════╝
`)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func initProviders() {
	// eSewa - auto-config sandbox
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
		log.Fatalf("eSewa init failed: %v", err)
	}
	log.Println("✓ eSewa initialized (sandbox)")

	// Khalti - requires env var
	if key := os.Getenv("KHALTI_SECRET_KEY"); key != "" {
		khaltiProvider, err = khalti.New(khalti.Config{
			SecretKey:  key,
			WebsiteURL: "http://localhost:8080",
			Sandbox:    true,
		})
		if err != nil {
			log.Printf("✗ Khalti init failed: %v", err)
		} else {
			log.Println("✓ Khalti initialized (sandbox)")
		}
	} else {
		log.Println("✗ Khalti skipped (set KHALTI_SECRET_KEY)")
	}

	// ConnectIPS - requires cert
	if os.Getenv("CONNECTIPS_MERCHANT_ID") != "" && os.Getenv("CONNECTIPS_CERT_PATH") != "" {
		connectipsProvider, err = connectips.New(connectips.Config{
			MerchantID:  os.Getenv("CONNECTIPS_MERCHANT_ID"),
			AppID:       os.Getenv("CONNECTIPS_APP_ID"),
			AppName:     os.Getenv("CONNECTIPS_APP_NAME"),
			AppPassword: os.Getenv("CONNECTIPS_APP_PASSWORD"),
			CertPath:    os.Getenv("CONNECTIPS_CERT_PATH"),
			Sandbox:     true,
		})
		if err != nil {
			log.Printf("✗ ConnectIPS init failed: %v", err)
		} else {
			log.Println("✓ ConnectIPS initialized (sandbox)")
		}
	} else {
		log.Println("✗ ConnectIPS skipped (set CONNECTIPS_* env vars)")
	}

	// FonePay - auto-config sandbox
	merchantCode := os.Getenv("FONEPAY_MERCHANT_CODE")
	fonepaySecret := os.Getenv("FONEPAY_SECRET_KEY")
	if merchantCode == "" {
		merchantCode = "NBQM"
	}
	if fonepaySecret == "" {
		fonepaySecret = "a7e3512f5032480a83137793cb2021dc"
	}
	fonepayProvider, err = fonepay.New(fonepay.Config{
		MerchantCode: merchantCode,
		SecretKey:    fonepaySecret,
		Sandbox:      merchantCode == "NBQM",
	})
	if err != nil {
		log.Printf("✗ FonePay init failed: %v", err)
	} else {
		log.Println("✓ FonePay initialized (sandbox)")
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
	resp, err := esewaProvider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      10000, // paisa (Rs 100)
		OrderID:     fmt.Sprintf("ESEWA-%d", time.Now().UnixNano()),
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
		http.Error(w, "Khalti not configured. Set KHALTI_SECRET_KEY.", http.StatusServiceUnavailable)
		return
	}
	resp, err := khaltiProvider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      10000, // paisa (Rs 100)
		OrderID:     fmt.Sprintf("KHALTI-%d", time.Now().UnixNano()),
		Description: "Premium Subscription",
		CallbackURL: "http://localhost:8080/payment/khalti/callback",
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Khalti initiation failed: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, resp.TargetURL, http.StatusFound)
}

func handleConnectIPSCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if connectipsProvider == nil {
		http.Error(w, "ConnectIPS not configured. Set CONNECTIPS_* env vars.", http.StatusServiceUnavailable)
		return
	}
	resp, err := connectipsProvider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      10000, // paisa (Rs 100)
		OrderID:     fmt.Sprintf("CIPS-%d", time.Now().UnixNano()),
		Description: "Premium Subscription",
		CallbackURL: "http://localhost:8080/payment/connectips/callback",
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("ConnectIPS initiation failed: %v", err), http.StatusInternalServerError)
		return
	}
	renderFormPOST(w, resp.TargetURL, resp.FormFields)
}

func handleFonepayCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	resp, err := fonepayProvider.InitiatePayment(r.Context(), &core.PaymentRequest{
		Amount:      10000, // paisa (Rs 100)
		OrderID:     fmt.Sprintf("FP-%d", time.Now().UnixNano()),
		Description: "Premium Subscription",
		CallbackURL: "http://localhost:8080/payment/fonepay/callback",
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("FonePay initiation failed: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, resp.TargetURL, http.StatusFound)
}

func handleEsewaCallback(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r, esewaProvider, 10000, "eSewa")
}

func handleEsewaFailure(w http.ResponseWriter, r *http.Request) {
	renderResult(w, false, "Payment Failed", "The eSewa payment was not completed.", nil)
}

func handleKhaltiCallback(w http.ResponseWriter, r *http.Request) {
	params := queryToMap(r)
	if params["status"] == "User canceled" {
		renderResult(w, false, "Payment Cancelled", "You cancelled the Khalti payment.", params)
		return
	}
	handleCallback(w, r, khaltiProvider, 10000, "Khalti")
}

func handleConnectIPSCallback(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r, connectipsProvider, 10000, "ConnectIPS")
}

func handleFonepayCallback(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r, fonepayProvider, 10000, "FonePay")
}

func handleCallback(w http.ResponseWriter, r *http.Request, p core.Provider, amount int64, name string) {
	log.Printf("%s callback: %v", name, r.URL.Query())
	params := queryToMap(r)

	result, err := p.VerifyPayment(r.Context(), &core.VerifyRequest{
		EncodedParams:  params,
		ExpectedAmount: amount,
	})
	if err != nil {
		log.Printf("%s verification error: %v", name, err)
		msg := "Payment verification failed"
		if errors.Is(err, core.ErrAmountMismatch) {
			msg = "Amount mismatch detected!"
		} else if errors.Is(err, core.ErrSignatureMismatch) {
			msg = "Signature verification failed!"
		}
		renderResult(w, false, "Verification Failed", msg, params)
		return
	}

	if result.Success {
		renderResult(w, true, "Payment Successful!",
			fmt.Sprintf("Provider: %s\nTransaction ID: %s\nAmount: NPR %.2f",
				name, result.TransactionID, float64(result.VerifiedAmount)/100), params)
	} else {
		renderResult(w, false, "Payment Not Completed",
			fmt.Sprintf("Status: %s", result.Status), params)
	}
}

func queryToMap(r *http.Request) map[string]string {
	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		params[key] = values[0]
	}
	return params
}

func renderFormPOST(w http.ResponseWriter, targetURL string, fields map[string]string) {
	tmpl := template.Must(template.New("form").Parse(`<!DOCTYPE html>
<html>
<head><title>Redirecting...</title></head>
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
	_ = tmpl.Execute(w, map[string]any{"URL": targetURL, "Fields": fields}) //nolint:errcheck
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
        a { color: #007bff; }
        .msg { white-space: pre-line; }
    </style>
</head>
<body>
    <div class="card">
        <h1>{{.Title}}</h1>
        <p class="msg">{{.Message}}</p>
        <p><a href="/">Back to Store</a></p>
    </div>
    {{if .Params}}<details><summary>Debug: Callback Parameters</summary>
    <div class="params"><pre>{{.Params}}</pre></div></details>{{end}}
</body>
</html>`))

	var sb strings.Builder
	for k, v := range params {
		fmt.Fprintf(&sb, "%s: %s\n", k, v)
	}
	w.Header().Set("Content-Type", "text/html")
	_ = tmpl.Execute(w, map[string]any{ //nolint:errcheck
		"Title":   title,
		"Message": message,
		"Color":   color,
		"Params":  sb.String(),
	})
}
