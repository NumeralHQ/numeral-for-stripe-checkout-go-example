package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NumeralHQ/numeral-for-stripe-checkout-go-example/internal/checkout"
	numeraltax "github.com/NumeralHQ/numeral-tax-go"
)

const publicIPLookupURL = "https://api.ipify.org?format=json"

type Config struct {
	NumeralAPIKey           string
	ConfigID                string
	RecurringPriceID        string
	Port                    string
	NumeralAPIBaseURL       string
	NumeralCheckoutURL      string
	NumeralElementScriptURL string
	TrustProxyHeaders       bool
	DisplayPriceCents       int64
	DisplayBillingInterval  string
}

type SessionCreator interface {
	CreateWithAddress(context.Context, checkout.AddressInput) (*numeraltax.TaxBridgeSessionNewResponse, error)
	CreateWithIP(context.Context, checkout.IPInput) (*numeraltax.TaxBridgeSessionNewResponse, error)
	CreateHosted(context.Context, checkout.CollectorInput) (*numeraltax.TaxBridgeSessionNewResponse, error)
	CreateEmbedded(context.Context, checkout.CollectorInput) (*numeraltax.TaxBridgeSessionNewResponse, error)
}

type application struct {
	config   Config
	sessions SessionCreator
	web      fs.FS
	http     *http.Client
}

type checkoutRequest struct {
	AddressCollection string `json:"addressCollection"`
	IPAddress         string `json:"ipAddress"`
	Country           string `json:"country"`
	Line1             string `json:"line1"`
	Line2             string `json:"line2"`
	City              string `json:"city"`
	Province          string `json:"province"`
	PostalCode        string `json:"postalCode"`
}

type checkoutResponse struct {
	Mode             string `json:"mode"`
	URL              string `json:"url,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	ClientSecret     string `json:"clientSecret,omitempty"`
	APIBase          string `json:"apiBase,omitempty"`
	CollectorURL     string `json:"collectorUrl,omitempty"`
	ElementScriptURL string `json:"elementScriptUrl,omitempty"`
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		NumeralAPIKey:          strings.TrimSpace(os.Getenv("NUMERAL_API_KEY")),
		ConfigID:               strings.TrimSpace(os.Getenv("NUMERAL_STRIPE_CHECKOUT_CONFIG_ID")),
		RecurringPriceID:       strings.TrimSpace(os.Getenv("STRIPE_RECURRING_PRICE_ID")),
		Port:                   envOrDefault("PORT", "3004"),
		NumeralAPIBaseURL:      withoutTrailingSlash(envOrDefault("NUMERAL_API_BASE_URL", "https://api.numeralhq.com")),
		NumeralCheckoutURL:     withoutTrailingSlash(envOrDefault("NUMERAL_CHECKOUT_URL", "https://checkout.numeralhq.com")),
		TrustProxyHeaders:      strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS")), "true"),
		DisplayPriceCents:      900,
		DisplayBillingInterval: strings.TrimSpace(envOrDefault("DEMO_BILLING_INTERVAL", "month")),
	}
	config.NumeralAPIBaseURL = strings.TrimSuffix(config.NumeralAPIBaseURL, "/tax")
	config.NumeralElementScriptURL = strings.TrimSpace(os.Getenv("NUMERAL_CHECKOUT_ELEMENT_URL"))
	if config.NumeralElementScriptURL == "" {
		config.NumeralElementScriptURL = config.NumeralCheckoutURL + "/bridge/v1.js"
	}
	if raw := strings.TrimSpace(os.Getenv("DEMO_PRICE_CENTS")); raw != "" {
		price, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || price < 0 {
			return Config{}, errors.New("DEMO_PRICE_CENTS must be a non-negative integer")
		}
		config.DisplayPriceCents = price
	}

	var missing []string
	if config.NumeralAPIKey == "" {
		missing = append(missing, "NUMERAL_API_KEY")
	}
	if config.ConfigID == "" {
		missing = append(missing, "NUMERAL_STRIPE_CHECKOUT_CONFIG_ID")
	}
	if config.RecurringPriceID == "" {
		missing = append(missing, "STRIPE_RECURRING_PRICE_ID")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return config, nil
}

func New(config Config, sessions SessionCreator, assets fs.FS) (http.Handler, error) {
	web, err := fs.Sub(assets, "web")
	if err != nil {
		return nil, err
	}
	app := &application{
		config:   config,
		sessions: sessions,
		web:      web,
		http:     &http.Client{Timeout: 5 * time.Second},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/checkout", app.createCheckout)
	mux.HandleFunc("GET /", app.index)
	mux.HandleFunc("GET /success", app.success)
	mux.HandleFunc("GET /cancel", app.cancel)
	mux.Handle("GET /styles.css", http.FileServerFS(web))
	mux.Handle("GET /app.js", http.FileServerFS(web))
	mux.Handle("GET /favicon.svg", http.FileServerFS(web))
	return securityHeaders(mux), nil
}

func (a *application) index(w http.ResponseWriter, _ *http.Request) {
	body, err := fs.ReadFile(a.web, "index.html")
	if err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
		return
	}
	page := strings.ReplaceAll(string(body), "{{PRICE_CENTS}}", strconv.FormatInt(a.config.DisplayPriceCents, 10))
	page = strings.ReplaceAll(page, "{{BILLING_INTERVAL}}", a.config.DisplayBillingInterval)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(page))
}

func (a *application) success(w http.ResponseWriter, _ *http.Request) {
	a.serveResult(w, "success.html")
}

func (a *application) cancel(w http.ResponseWriter, _ *http.Request) {
	a.serveResult(w, "cancel.html")
}

func (a *application) serveResult(w http.ResponseWriter, name string) {
	body, err := fs.ReadFile(a.web, name)
	if err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (a *application) createCheckout(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	var input checkoutRequest
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid checkout request.")
		return
	}

	input.AddressCollection = strings.TrimSpace(input.AddressCollection)
	input.Country = strings.ToUpper(strings.TrimSpace(input.Country))
	if input.Country == "" {
		input.Country = "US"
	}
	input.Province = strings.ToUpper(strings.TrimSpace(input.Province))
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.IPAddress = strings.TrimSpace(input.IPAddress)

	if !validCollectionMode(input.AddressCollection) {
		writeError(w, http.StatusBadRequest, "Choose a tax-location option.")
		return
	}
	if input.AddressCollection == "merchant" && !validUSAddress(input) {
		writeError(w, http.StatusBadRequest, "Please complete the billing address with a valid state and ZIP code.")
		return
	}

	ipAddress := input.IPAddress
	if input.AddressCollection == "device_ip" {
		ipAddress = a.detectDeviceIP(r)
	}
	if (input.AddressCollection == "customer_ip" || input.AddressCollection == "device_ip") && !usablePublicIP(ipAddress) {
		message := "Enter the customer public IP address your server received."
		if input.AddressCollection == "device_ip" {
			message = "Northstar could not detect this device's public IP. Try the Customer IP option instead."
		}
		writeError(w, http.StatusBadRequest, message)
		return
	}

	idempotencyKey, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start checkout.")
		return
	}
	origin := requestOrigin(r, a.config.TrustProxyHeaders)
	common := checkout.CommonInput{
		SuccessURL:        origin + "/success",
		CancelURL:         origin + "/cancel",
		IdempotencyKey:    idempotencyKey,
		ExternalReference: "northstar_go_subscription_demo:" + idempotencyKey,
	}

	var session *numeraltax.TaxBridgeSessionNewResponse
	switch input.AddressCollection {
	case "merchant":
		session, err = a.sessions.CreateWithAddress(r.Context(), checkout.AddressInput{
			CommonInput: common,
			Address: checkout.Address{
				Country:    input.Country,
				Line1:      strings.TrimSpace(input.Line1),
				Line2:      strings.TrimSpace(input.Line2),
				City:       strings.TrimSpace(input.City),
				Province:   input.Province,
				PostalCode: input.PostalCode,
			},
		})
	case "customer_ip", "device_ip":
		session, err = a.sessions.CreateWithIP(r.Context(), checkout.IPInput{
			CommonInput: common,
			IPAddress:   ipAddress,
		})
	case "numeral_hosted":
		session, err = a.sessions.CreateHosted(r.Context(), checkout.CollectorInput{CommonInput: common, Country: input.Country})
	case "numeral_embedded":
		session, err = a.sessions.CreateEmbedded(r.Context(), checkout.CollectorInput{CommonInput: common, Country: input.Country})
	}
	if err != nil {
		slog.Error("Numeral subscription checkout failed", "error", err, "collection_mode", input.AddressCollection)
		writeError(w, http.StatusBadGateway, "Numeral could not prepare the subscription checkout. Check the server log for details.")
		return
	}
	if session == nil {
		writeError(w, http.StatusBadGateway, "Numeral returned an empty checkout session.")
		return
	}

	if input.AddressCollection == "numeral_embedded" {
		if session.ClientSecret == "" {
			writeError(w, http.StatusBadGateway, "Numeral did not return an embedded collector capability.")
			return
		}
		writeJSON(w, http.StatusOK, checkoutResponse{
			Mode:             "embedded",
			SessionID:        session.ID,
			ClientSecret:     session.ClientSecret,
			APIBase:          a.config.NumeralAPIBaseURL + "/tax",
			CollectorURL:     a.config.NumeralCheckoutURL,
			ElementScriptURL: a.config.NumeralElementScriptURL,
		})
		return
	}
	if session.URL == "" {
		writeError(w, http.StatusBadGateway, "Numeral did not return a checkout URL.")
		return
	}
	writeJSON(w, http.StatusOK, checkoutResponse{Mode: "redirect", URL: session.URL, SessionID: session.ID})
}

func (a *application) detectDeviceIP(r *http.Request) string {
	if a.config.TrustProxyHeaders {
		for _, candidate := range []string{
			r.Header.Get("CF-Connecting-IP"),
			strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0],
			r.Header.Get("X-Real-IP"),
		} {
			candidate = strings.TrimSpace(candidate)
			if usablePublicIP(candidate) {
				return candidate
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && usablePublicIP(host) {
		return host
	}

	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, publicIPLookupURL, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Accept", "application/json")
	response, err := a.http.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	var data struct {
		IP string `json:"ip"`
	}
	if json.NewDecoder(response.Body).Decode(&data) != nil || !usablePublicIP(data.IP) {
		return ""
	}
	return data.IP
}

func validCollectionMode(value string) bool {
	switch value {
	case "merchant", "customer_ip", "device_ip", "numeral_embedded", "numeral_hosted":
		return true
	default:
		return false
	}
}

func validUSAddress(input checkoutRequest) bool {
	if input.Country != "US" || len(input.Province) != 2 {
		return false
	}
	postalCode := input.PostalCode
	validPostalCode := len(postalCode) == 5
	if len(postalCode) == 10 && postalCode[5] == '-' {
		validPostalCode = true
	}
	for index, character := range postalCode {
		if index == 5 && len(postalCode) == 10 {
			continue
		}
		if character < '0' || character > '9' {
			validPostalCode = false
		}
	}
	return strings.TrimSpace(input.Line1) != "" && strings.TrimSpace(input.City) != "" && validPostalCode
}

func usablePublicIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast()
}

func requestOrigin(r *http.Request, trustProxy bool) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
	}
	return scheme + "://" + r.Host
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func withoutTrailingSlash(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
