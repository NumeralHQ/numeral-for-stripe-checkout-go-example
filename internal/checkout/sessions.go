package checkout

import (
	"context"

	numeraltax "github.com/NumeralHQ/numeral-tax-go"
)

type ServiceConfig struct {
	ConfigID         string
	RecurringPriceID string
	ProductCategory  string
}

type Service struct {
	client numeraltax.Client
	config ServiceConfig
}

type CommonInput struct {
	SuccessURL        string
	CancelURL         string
	IdempotencyKey    string
	ExternalReference string
}

type Address struct {
	Country    string
	Line1      string
	Line2      string
	City       string
	Province   string
	PostalCode string
}

type AddressInput struct {
	CommonInput
	Address Address
}

type IPInput struct {
	CommonInput
	IPAddress string
}

type CollectorInput struct {
	CommonInput
	Country string
}

func NewService(client numeraltax.Client, config ServiceConfig) *Service {
	return &Service{client: client, config: config}
}

// CreateWithAddress sends a complete customer address. Numeral calculates tax
// and normally returns a Stripe Checkout URL without collecting more input.
func (s *Service) CreateWithAddress(ctx context.Context, input AddressInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	address := numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObjectAddress{
		Country:    input.Address.Country,
		Line1:      numeraltax.String(input.Address.Line1),
		City:       numeraltax.String(input.Address.City),
		Province:   numeraltax.String(input.Address.Province),
		PostalCode: numeraltax.String(input.Address.PostalCode),
	}
	if input.Address.Line2 != "" {
		address.Line2 = numeraltax.String(input.Address.Line2)
	}

	return s.client.Tax.Bridge.Sessions.New(ctx, numeraltax.TaxBridgeSessionNewParams{
		XAPIVersion:        numeraltax.TaxBridgeSessionNewParamsXAPIVersion2026_03_01,
		IdempotencyKey:     numeraltax.String(input.IdempotencyKey),
		ConfigID:           s.config.ConfigID,
		CollectionMode:     numeraltax.TaxBridgeSessionNewParamsCollectionModeHosted,
		ConfirmationMethod: numeraltax.TaxBridgeSessionNewParamsConfirmationMethodAutomatic,
		ExternalReference:  numeraltax.String(input.ExternalReference),
		Checkout: numeraltax.TaxBridgeSessionNewParamsCheckout{
			Mode: "subscription",
			LineItems: []numeraltax.TaxBridgeSessionNewParamsCheckoutLineItem{{
				Price:           s.config.RecurringPriceID,
				Quantity:        1,
				ProductCategory: numeraltax.String(s.config.ProductCategory),
			}},
			SuccessURL: numeraltax.String(input.SuccessURL),
			CancelURL:  numeraltax.String(input.CancelURL),
		},
		TaxContext: numeraltax.TaxBridgeSessionNewParamsTaxContext{
			Location: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationUnion{
				OfTaxBridgeSessionNewsTaxContextLocationObject: &numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObject{
					Basis:     "billing_address",
					Assurance: "self_attested",
					Address:   address,
				},
			},
		},
	})
}

// CreateWithIP sends the customer's public IP. Numeral resolves it and only
// collects more address information when the resolved location is insufficient.
func (s *Service) CreateWithIP(ctx context.Context, input IPInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	return s.client.Tax.Bridge.Sessions.New(ctx, numeraltax.TaxBridgeSessionNewParams{
		XAPIVersion:        numeraltax.TaxBridgeSessionNewParamsXAPIVersion2026_03_01,
		IdempotencyKey:     numeraltax.String(input.IdempotencyKey),
		ConfigID:           s.config.ConfigID,
		CollectionMode:     numeraltax.TaxBridgeSessionNewParamsCollectionModeHosted,
		ConfirmationMethod: numeraltax.TaxBridgeSessionNewParamsConfirmationMethodAutomatic,
		ExternalReference:  numeraltax.String(input.ExternalReference),
		Checkout: numeraltax.TaxBridgeSessionNewParamsCheckout{
			Mode: "subscription",
			LineItems: []numeraltax.TaxBridgeSessionNewParamsCheckoutLineItem{{
				Price:           s.config.RecurringPriceID,
				Quantity:        1,
				ProductCategory: numeraltax.String(s.config.ProductCategory),
			}},
			SuccessURL: numeraltax.String(input.SuccessURL),
			CancelURL:  numeraltax.String(input.CancelURL),
		},
		TaxContext: numeraltax.TaxBridgeSessionNewParamsTaxContext{
			Location: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationUnion{
				OfTaxBridgeSessionNewsTaxContextLocationObject2: &numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObject2{
					Basis: "billing_address",
					IP: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObject2IP{
						Value: input.IPAddress,
					},
				},
			},
		},
	})
}

// CreateHosted lets Numeral host the missing-address step before redirecting
// the buyer to Stripe Checkout.
func (s *Service) CreateHosted(ctx context.Context, input CollectorInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	return s.client.Tax.Bridge.Sessions.New(ctx, numeraltax.TaxBridgeSessionNewParams{
		XAPIVersion:        numeraltax.TaxBridgeSessionNewParamsXAPIVersion2026_03_01,
		IdempotencyKey:     numeraltax.String(input.IdempotencyKey),
		ConfigID:           s.config.ConfigID,
		CollectionMode:     numeraltax.TaxBridgeSessionNewParamsCollectionModeHosted,
		ConfirmationMethod: numeraltax.TaxBridgeSessionNewParamsConfirmationMethodAutomatic,
		ExternalReference:  numeraltax.String(input.ExternalReference),
		Checkout: numeraltax.TaxBridgeSessionNewParamsCheckout{
			Mode: "subscription",
			LineItems: []numeraltax.TaxBridgeSessionNewParamsCheckoutLineItem{{
				Price:           s.config.RecurringPriceID,
				Quantity:        1,
				ProductCategory: numeraltax.String(s.config.ProductCategory),
			}},
			SuccessURL: numeraltax.String(input.SuccessURL),
			CancelURL:  numeraltax.String(input.CancelURL),
		},
		TaxContext: numeraltax.TaxBridgeSessionNewParamsTaxContext{
			Location: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationUnion{
				OfTaxBridgeSessionNewsTaxContextLocationObject: &numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObject{
					Basis:     "billing_address",
					Assurance: "self_attested",
					Address: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObjectAddress{
						Country: input.Country,
					},
				},
			},
		},
	})
}

// CreateEmbedded returns a one-time client secret for the
// <numeral-checkout> element on the merchant's checkout page.
func (s *Service) CreateEmbedded(ctx context.Context, input CollectorInput) (*numeraltax.TaxBridgeSessionNewResponse, error) {
	return s.client.Tax.Bridge.Sessions.New(ctx, numeraltax.TaxBridgeSessionNewParams{
		XAPIVersion:        numeraltax.TaxBridgeSessionNewParamsXAPIVersion2026_03_01,
		IdempotencyKey:     numeraltax.String(input.IdempotencyKey),
		ConfigID:           s.config.ConfigID,
		CollectionMode:     numeraltax.TaxBridgeSessionNewParamsCollectionModeEmbedded,
		ConfirmationMethod: numeraltax.TaxBridgeSessionNewParamsConfirmationMethodAutomatic,
		ExternalReference:  numeraltax.String(input.ExternalReference),
		Checkout: numeraltax.TaxBridgeSessionNewParamsCheckout{
			Mode: "subscription",
			LineItems: []numeraltax.TaxBridgeSessionNewParamsCheckoutLineItem{{
				Price:           s.config.RecurringPriceID,
				Quantity:        1,
				ProductCategory: numeraltax.String(s.config.ProductCategory),
			}},
			SuccessURL: numeraltax.String(input.SuccessURL),
			CancelURL:  numeraltax.String(input.CancelURL),
		},
		TaxContext: numeraltax.TaxBridgeSessionNewParamsTaxContext{
			Location: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationUnion{
				OfTaxBridgeSessionNewsTaxContextLocationObject: &numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObject{
					Basis:     "billing_address",
					Assurance: "self_attested",
					Address: numeraltax.TaxBridgeSessionNewParamsTaxContextLocationObjectAddress{
						Country: input.Country,
					},
				},
			},
		},
	})
}
