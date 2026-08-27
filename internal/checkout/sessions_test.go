package checkout

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	numeraltax "github.com/NumeralHQ/numeral-tax-go"
	"github.com/NumeralHQ/numeral-tax-go/option"
)

func TestSubscriptionCreationPaths(t *testing.T) {
	t.Parallel()

	type capturedRequest struct {
		header http.Header
		body   map[string]any
	}
	captured := make(chan capturedRequest, 4)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Errorf("decode request: %v", err)
		}
		captured <- capturedRequest{header: r.Header.Clone(), body: decoded}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"brs_test","url":"https://checkout.stripe.com/test","client_secret":"example_client_secret"}`))
	}))
	t.Cleanup(api.Close)

	client := numeraltax.NewClient(
		option.WithAPIKey("sk_test_example"),
		option.WithBaseURL(api.URL+"/"),
	)
	service := NewService(client, ServiceConfig{
		ConfigID:         "brcfg_test",
		RecurringPriceID: "price_recurring_test",
		ProductCategory:  "GENERAL_MERCHANDISE",
	})
	common := CommonInput{
		SuccessURL:        "http://localhost:3004/success",
		CancelURL:         "http://localhost:3004/cancel",
		IdempotencyKey:    "idem_test",
		ExternalReference: "subscription_test",
	}

	tests := []struct {
		name           string
		collectionMode string
		create         func() error
		assertLocation func(*testing.T, map[string]any)
	}{
		{
			name:           "complete address",
			collectionMode: "hosted",
			create: func() error {
				_, err := service.CreateWithAddress(context.Background(), AddressInput{
					CommonInput: common,
					Address:     Address{Country: "US", Line1: "123 Main St", City: "New York", Province: "NY", PostalCode: "10001"},
				})
				return err
			},
			assertLocation: func(t *testing.T, location map[string]any) {
				t.Helper()
				address := object(t, location, "address")
				if address["postal_code"] != "10001" || address["province"] != "NY" {
					t.Fatalf("unexpected address: %#v", address)
				}
			},
		},
		{
			name:           "customer IP",
			collectionMode: "hosted",
			create: func() error {
				_, err := service.CreateWithIP(context.Background(), IPInput{CommonInput: common, IPAddress: "76.121.4.10"})
				return err
			},
			assertLocation: func(t *testing.T, location map[string]any) {
				t.Helper()
				if object(t, location, "ip")["value"] != "76.121.4.10" {
					t.Fatalf("unexpected IP location: %#v", location)
				}
			},
		},
		{
			name:           "hosted collector",
			collectionMode: "hosted",
			create: func() error {
				_, err := service.CreateHosted(context.Background(), CollectorInput{CommonInput: common, Country: "US"})
				return err
			},
			assertLocation: countryOnlyLocation,
		},
		{
			name:           "embedded collector",
			collectionMode: "embedded",
			create: func() error {
				_, err := service.CreateEmbedded(context.Background(), CollectorInput{CommonInput: common, Country: "US"})
				return err
			},
			assertLocation: countryOnlyLocation,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.create(); err != nil {
				t.Fatalf("create session: %v", err)
			}
			request := <-captured
			if request.header.Get("X-API-Version") != "2026-03-01" {
				t.Fatalf("X-API-Version = %q", request.header.Get("X-API-Version"))
			}
			if request.header.Get("Idempotency-Key") != "idem_test" {
				t.Fatalf("Idempotency-Key = %q", request.header.Get("Idempotency-Key"))
			}
			if request.body["config_id"] != "brcfg_test" || request.body["collection_mode"] != test.collectionMode {
				t.Fatalf("unexpected session envelope: %#v", request.body)
			}
			checkoutBody := object(t, request.body, "checkout")
			if checkoutBody["mode"] != "subscription" {
				t.Fatalf("checkout mode = %#v", checkoutBody["mode"])
			}
			lineItems, ok := checkoutBody["line_items"].([]any)
			if !ok || len(lineItems) != 1 || lineItems[0].(map[string]any)["price"] != "price_recurring_test" {
				t.Fatalf("unexpected line items: %#v", checkoutBody["line_items"])
			}
			taxContext := object(t, request.body, "tax_context")
			test.assertLocation(t, object(t, taxContext, "location"))
		})
	}
}

func countryOnlyLocation(t *testing.T, location map[string]any) {
	t.Helper()
	address := object(t, location, "address")
	if address["country"] != "US" || len(address) != 1 {
		t.Fatalf("expected country-only address, got %#v", address)
	}
}

func object(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %#v", key, parent[key])
	}
	return value
}
