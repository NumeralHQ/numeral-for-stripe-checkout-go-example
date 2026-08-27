package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NumeralHQ/numeral-for-stripe-checkout-go-example/internal/checkout"
	numeraltax "github.com/NumeralHQ/numeral-tax-go"
)

type fakeSessions struct {
	lastPath string
}

func (f *fakeSessions) CreateWithAddress(context.Context, checkout.AddressInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	f.lastPath = "address"
	return &numeraltax.TaxBridgeSessionNewResponse{ID: "brs_address", URL: "https://checkout.stripe.com/address"}, nil
}

func (f *fakeSessions) CreateWithIP(context.Context, checkout.IPInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	f.lastPath = "ip"
	return &numeraltax.TaxBridgeSessionNewResponse{ID: "brs_ip", URL: "https://checkout.stripe.com/ip"}, nil
}

func (f *fakeSessions) CreateHosted(context.Context, checkout.CollectorInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	f.lastPath = "hosted"
	return &numeraltax.TaxBridgeSessionNewResponse{ID: "brs_hosted", URL: "https://checkout.numeralhq.com/s/brs_hosted"}, nil
}

func (f *fakeSessions) CreateEmbedded(context.Context, checkout.CollectorInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	f.lastPath = "embedded"
	return &numeraltax.TaxBridgeSessionNewResponse{ID: "brs_embedded", ClientSecret: "example_client_secret"}, nil
}

func TestPageShowsFiveSubscriptionPaths(t *testing.T) {
	handler := testHandler(t, &fakeSessions{})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, expected := range []string{"Address", "Customer IP", "My IP", "Embedded", "Hosted", "Recurring subscription"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("page does not contain %q", expected)
		}
	}
	if strings.Contains(body, "{{PRICE_CENTS}}") || !strings.Contains(body, "data-price-cents=\"900\"") {
		t.Fatal("display price template was not rendered")
	}
}

func TestEmbeddedCheckoutResponse(t *testing.T) {
	sessions := &fakeSessions{}
	handler := testHandler(t, sessions)
	body := bytes.NewBufferString(`{"addressCollection":"numeral_embedded","country":"US"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/checkout", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		result, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.Code, result)
	}
	if sessions.lastPath != "embedded" {
		t.Fatalf("path = %q", sessions.lastPath)
	}
	result := response.Body.String()
	for _, expected := range []string{`"mode":"embedded"`, `"clientSecret":"example_client_secret"`, `"sessionId":"brs_embedded"`} {
		if !strings.Contains(result, expected) {
			t.Fatalf("response does not contain %s: %s", expected, result)
		}
	}
}

func TestUsablePublicIP(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.2", "::1", "not-an-ip"} {
		if usablePublicIP(value) {
			t.Errorf("usablePublicIP(%q) = true", value)
		}
	}
	for _, value := range []string{"76.121.4.10", "8.8.8.8", "2606:4700:4700::1111"} {
		if !usablePublicIP(value) {
			t.Errorf("usablePublicIP(%q) = false", value)
		}
	}
}

func testHandler(t *testing.T, sessions SessionCreator) http.Handler {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	assets := os.DirFS(strings.TrimSuffix(workingDirectory, "/internal/server"))
	handler, err := New(Config{
		NumeralAPIBaseURL:       "https://api.numeralhq.com",
		NumeralCheckoutURL:      "https://checkout.numeralhq.com",
		NumeralElementScriptURL: "https://checkout.numeralhq.com/bridge/v1.js",
		DisplayPriceCents:       900,
		DisplayBillingInterval:  "month",
	}, sessions, assets)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
