# Numeral for Stripe Checkout subscriptions — Go example

This repository is a test-mode subscription checkout for a fictional merchant
named Northstar. It uses the published
[`numeral-tax-go`](https://pkg.go.dev/github.com/NumeralHQ/numeral-tax-go)
SDK to ask Numeral to validate the customer's tax location, calculate tax, and
prepare a Stripe Checkout **subscription** session.

The entire example is Go: one server embeds and serves the browser UI, creates
Bridge sessions with the Go SDK, and runs at
[http://localhost:3004](http://localhost:3004). No Node runtime is required.

## Five subscription paths

1. **Merchant address** — send a complete address and normally go directly to
   Stripe.
2. **Customer IP** — send a public customer IP and let Numeral resolve it.
3. **My device IP** — read the buyer IP from the incoming Go server request.
4. **Numeral embedded** — collect missing address fields inside the merchant
   page.
5. **Numeral hosted** — redirect through Numeral's hosted address step.

Every path creates the same recurring subscription by sending:

```go
Checkout: numeraltax.TaxBridgeSessionNewParamsCheckout{
    Mode: "subscription",
    LineItems: []numeraltax.TaxBridgeSessionNewParamsCheckoutLineItem{{
        Price:    recurringPriceID,
        Quantity: 1,
    }},
}
```

The Numeral API key stays on the Go server. The embedded browser component only
receives a short-lived capability bound to its Bridge session.

## Prerequisites

- Go 1.22 or newer
- A Numeral test-mode API key
- A test Stripe connection in Numeral
- A published Numeral for Stripe Checkout test configuration
- A **recurring** test Stripe Price from the connected Stripe account

## Configure Numeral

In the Numeral dashboard, enable **Test Mode**, then:

1. Confirm the test Stripe account under **Connections**.
2. Open **Developers → Numeral Stripe Checkout**.
3. Select that Stripe connection and complete the configuration.
4. Use `http://localhost:3004/success` as the success URL.
5. Use `http://localhost:3004/cancel` as the cancel URL.
6. Add `http://localhost:3004` to both **Allowed redirect origins** and
   **Allowed embed origins**.
7. Save and publish, then copy the resulting `brcfg_...` ID.

The Stripe Price must have a recurring interval. A one-time Price cannot be the
only line item in a Stripe Checkout subscription.

## Run locally

```bash
cp .env.example .env.local
```

Fill in the three test-mode values:

```dotenv
NUMERAL_API_KEY=sk_test_replace_me
NUMERAL_STRIPE_CHECKOUT_CONFIG_ID=brcfg_replace_me
STRIPE_RECURRING_PRICE_ID=price_replace_me
```

`STRIPE_RECURRING_PRICE_ID` must reference a recurring Price from the same
Stripe test account selected by the `brcfg_...` configuration. A Price created
for a one-time payment cannot be used with `checkout.mode: "subscription"`.

Then start the checkout:

```bash
go run .
```

Open [http://localhost:3004](http://localhost:3004). On Stripe Checkout, use
`4242 4242 4242 4242`, any future expiry date, and any CVC.

The example defaults to Numeral's production API and hosted collector while
using test-mode credentials and Stripe objects. The optional endpoint variables
in `.env.example` are only needed when developing Numeral itself.

If you set `NUMERAL_API_BASE_URL`, use the API origin
(`https://api.numeralhq.com`). The application also normalizes a legacy value
ending in `/tax` and explicitly passes the resulting origin to the generated Go
SDK. This prevents the SDK's `tax/bridge/...` endpoint paths from accidentally
becoming `/tax/tax/bridge/...`.

## Copyable Go SDK examples

The four server-side integration functions are in
[`internal/checkout/sessions.go`](internal/checkout/sessions.go):

- `CreateWithAddress` sends the complete customer address.
- `CreateWithIP` sends the customer's public IPv4 or IPv6 address.
- `CreateHosted` lets Numeral host the missing-address step.
- `CreateEmbedded` returns a one-time client secret for
  `<numeral-checkout>`.

Each function contains a complete
`client.Tax.Bridge.Sessions.New(ctx, params)` call with
`Mode: "subscription"`, so clients can copy the path they need without
unwinding conditional request construction. The HTTP handler in
[`internal/server/server.go`](internal/server/server.go) validates browser
input, chooses one function, and formats its response.

The embedded integration in [`web/app.js`](web/app.js) loads Numeral's
versioned browser script, creates `<numeral-checkout>`, and assigns the one-time
`client_secret` as a JavaScript property so it never appears in HTML, storage,
or a URL.

## Buyer IP guidance

Only send the customer's public IP. In production, derive it from trusted proxy
headers that your hosting platform overwrites. This example ignores forwarded
IP headers unless `TRUST_PROXY_HEADERS=true`; do not enable that setting unless
the server is actually behind a trusted reverse proxy.

For a convenient local demo, the **My IP** path falls back to a server-side
public-IP lookup when the direct request comes from localhost. If IP resolution
does not establish enough location information, Numeral uses the selected
collection mode to obtain the missing address fields.

## Verify the project

```bash
go test ./...
go vet ./...
go build ./...
```

The integration tests use a local fake Numeral API and verify that all four SDK
functions send `checkout.mode: "subscription"`, the recurring Price, API
version, idempotency key, collection mode, and expected tax-location shape.

## Deployment notes

This is an integration example, not a production storefront. Before deploying
it publicly, keep it on dedicated test Numeral and Stripe accounts, store all
credentials in server-only environment variables, and rate-limit
`POST /api/checkout`. Never place live keys in a public demo.
