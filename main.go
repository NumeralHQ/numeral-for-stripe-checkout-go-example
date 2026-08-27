package main

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/NumeralHQ/numeral-for-stripe-checkout-go-example/internal/checkout"
	"github.com/NumeralHQ/numeral-for-stripe-checkout-go-example/internal/server"
	numeraltax "github.com/NumeralHQ/numeral-tax-go"
	"github.com/NumeralHQ/numeral-tax-go/option"
	"github.com/joho/godotenv"
)

//go:embed web/*
var webAssets embed.FS

func main() {
	_ = godotenv.Load(".env.local")

	config, err := server.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	clientOptions := []option.RequestOption{option.WithAPIKey(config.NumeralAPIKey)}
	if config.NumeralAPIBaseURL != "https://api.numeralhq.com" {
		clientOptions = append(clientOptions, option.WithBaseURL(strings.TrimRight(config.NumeralAPIBaseURL, "/")+"/"))
	}
	client := numeraltax.NewClient(clientOptions...)
	sessions := checkout.NewService(client, checkout.ServiceConfig{
		ConfigID:         config.ConfigID,
		RecurringPriceID: config.RecurringPriceID,
		ProductCategory:  "GENERAL_MERCHANDISE",
	})

	handler, err := server.New(config, sessions, webAssets)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{
		Addr:              ":" + config.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Numeral for Stripe Checkout Go example: http://localhost:%s", config.Port)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
