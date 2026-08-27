const MODES = {
  merchant: {
    description: "Northstar already has the billing address, so Numeral can calculate tax before opening Stripe.",
    button: "Continue to subscribe",
  },
  customer_ip: {
    description: "Northstar sends the customer's public IP and Numeral resolves the tax location.",
    button: "Continue using IP",
  },
  device_ip: {
    description: "The Go server reads this buyer's public IP from the incoming request and sends it to Numeral.",
    button: "Continue with my IP",
    previewTitle: "No address entry required",
    previewCopy: "Northstar's Go server detects the buyer IP. Numeral only asks for an address if IP resolution is insufficient.",
  },
  numeral_embedded: {
    description: "Numeral securely collects the missing tax fields inside Northstar's subscription page.",
    button: "Continue with Numeral",
    previewTitle: "Numeral will collect the billing address",
    previewCopy: "The subscription session starts first; the embedded collector asks only for the fields tax calculation still needs.",
  },
  numeral_hosted: {
    description: "Numeral hosts the address step, then sends the prepared subscription to Stripe Checkout.",
    button: "Continue to Numeral",
    previewTitle: "Numeral will host the address step",
    previewCopy: "The buyer securely provides the missing tax fields on checkout.numeralhq.com before continuing to Stripe.",
  },
};

const form = document.querySelector("#checkout-form");
const buttons = [...document.querySelectorAll("[data-mode]")];
const addressFields = document.querySelector("#address-fields");
const customerIPFields = document.querySelector("#customer-ip-fields");
const preview = document.querySelector("#preview");
const previewTitle = document.querySelector("#preview-title");
const previewCopy = document.querySelector("#preview-copy");
const collectorSection = document.querySelector("#collector-section");
const collectorHost = document.querySelector("#collector-host");
const errorBox = document.querySelector("#checkout-error");
const errorMessage = document.querySelector("#checkout-error-message");
const submitButton = document.querySelector("#checkout-button");
const submitLabel = document.querySelector("#checkout-button-label");
const modeDescription = document.querySelector("#mode-description");
const priceCents = Number(document.body.dataset.priceCents || "900");
const billingInterval = document.body.dataset.billingInterval || "month";

let selectedMode = "merchant";
let status = "idle";
let collectorElement = null;
let taxCents = null;

function money(cents) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
}

function updateSummary() {
  document.querySelector("#product-price").textContent = money(priceCents);
  document.querySelector("#product-interval").textContent = ` / ${billingInterval}`;
  document.querySelector("#subtotal").textContent = money(priceCents);
  document.querySelector("#total-interval").textContent = billingInterval;
  document.querySelector("#total-due").textContent = taxCents === null ? `${money(priceCents)} + tax` : money(priceCents + taxCents);
  const tax = document.querySelector("#tax-summary");
  tax.textContent = taxCents === null ? "Calculated next" : money(taxCents);
  tax.classList.toggle("tax-pending", taxCents === null);
}

function setStatus(next) {
  status = next;
  const busy = next === "starting" || next === "redirecting";
  submitButton.disabled = busy;
  buttons.forEach((button) => (button.disabled = next !== "idle"));
  if (next === "starting") {
    submitLabel.textContent = selectedMode === "device_ip" ? "Detecting your IP…" : "Starting subscription…";
  } else if (next === "redirecting") {
    submitLabel.textContent = "Opening secure checkout…";
  } else {
    submitLabel.textContent = MODES[selectedMode].button;
  }
}

function showError(message) {
  errorMessage.textContent = message;
  errorBox.hidden = false;
}

function clearError() {
  errorBox.hidden = true;
  errorMessage.textContent = "";
}

function destroyCollector() {
  if (collectorElement) {
    collectorElement.destroy?.();
    collectorElement.remove();
    collectorElement = null;
  }
  collectorHost.replaceChildren();
  collectorSection.hidden = true;
}

function selectMode(mode) {
  if (status !== "idle" || !MODES[mode]) return;
  selectedMode = mode;
  buttons.forEach((button) => {
    const active = button.dataset.mode === mode;
    button.classList.toggle("active", active);
    button.setAttribute("aria-checked", String(active));
  });
  addressFields.hidden = mode !== "merchant";
  customerIPFields.hidden = mode !== "customer_ip";
  preview.hidden = mode === "merchant" || mode === "customer_ip";
  if (!preview.hidden) {
    previewTitle.textContent = MODES[mode].previewTitle;
    previewCopy.textContent = MODES[mode].previewCopy;
  }
  modeDescription.textContent = MODES[mode].description;
  taxCents = null;
  updateSummary();
  clearError();
  destroyCollector();
  setStatus("idle");
}

function collectorErrorMessage(code) {
  if (code === "collector_load_failed") return "The Numeral collector script could not load from its configured URL.";
  if (code === "origin_not_allowed") return "Add http://localhost:3004 to Allowed embed origins in Developers → Numeral Stripe Checkout, save and publish, then try again.";
  if (code === "zip_state_mismatch") return "The ZIP code and state do not match. Check both fields and try again.";
  return `Numeral's secure address step could not continue (${code}).`;
}

function eventCode(event) {
  const detail = event.detail;
  if (detail && typeof detail === "object" && typeof detail.code === "string") return detail.code;
  return "unknown_error";
}

function taxAmount(event) {
  const amount = event.detail?.totalTaxAmount;
  return typeof amount === "number" ? amount : null;
}

async function loadCollectorScript(src) {
  if (customElements.get("numeral-checkout")) return;
  const existing = document.querySelector(`script[data-numeral-element="${CSS.escape(src)}"]`);
  if (existing) {
    await customElements.whenDefined("numeral-checkout");
    return;
  }
  await new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.type = "module";
    script.src = src;
    script.dataset.numeralElement = src;
    script.addEventListener("load", resolve, { once: true });
    script.addEventListener("error", reject, { once: true });
    document.head.appendChild(script);
  });
  await customElements.whenDefined("numeral-checkout");
}

async function mountCollector(session) {
  collectorSection.hidden = false;
  preview.hidden = true;
  try {
    await loadCollectorScript(session.elementScriptUrl);
    const element = document.createElement("numeral-checkout");
    element.setAttribute("api-base", session.apiBase);
    element.setAttribute("collector-url", session.collectorUrl);
    element.setAttribute("session-id", session.sessionId);
    element.setAttribute("presentation", "inline");
    element.setAttribute("redirect", "auto");
    element.style.display = "block";
    element.style.setProperty("--numeral-bridge-color-primary", "#173f2d");
    element.style.setProperty("--numeral-bridge-font-family", "Inter, ui-sans-serif, system-ui, sans-serif");
    element.style.setProperty("--numeral-bridge-border-radius", "10px");

    // Keep the one-time capability in memory. Never put it in an attribute,
    // page markup, storage, or a URL.
    element.clientSecret = session.clientSecret;
    element.addEventListener("numeral-error", (event) => {
      showError(collectorErrorMessage(eventCode(event)));
      setStatus("collecting");
    });
    element.addEventListener("numeral-before-redirect", () => setStatus("redirecting"));
    element.addEventListener("numeral-cancel", () => {
      destroyCollector();
      preview.hidden = false;
      setStatus("idle");
    });
    element.addEventListener("numeral-tax-calculated", (event) => {
      const amount = taxAmount(event);
      if (amount !== null) {
        taxCents = amount;
        updateSummary();
      }
    });
    collectorHost.appendChild(element);
    collectorElement = element;
    await element.open();
  } catch {
    showError(collectorErrorMessage("collector_load_failed"));
    setStatus("collecting");
  }
}

buttons.forEach((button) => button.addEventListener("click", () => selectMode(button.dataset.mode)));
form.addEventListener("input", clearError);
form.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearError();
  taxCents = null;
  updateSummary();
  setStatus("starting");

  const values = Object.fromEntries(new FormData(form).entries());
  try {
    const response = await fetch("/api/checkout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...values, addressCollection: selectedMode }),
    });
    const data = await response.json();
    if (!response.ok || data.error) throw new Error(data.error || "We could not start the subscription checkout.");
    if (data.mode === "embedded") {
      setStatus("collecting");
      await mountCollector(data);
      return;
    }
    setStatus("redirecting");
    window.setTimeout(() => window.location.assign(data.url), 350);
  } catch (error) {
    setStatus("idle");
    showError(error instanceof Error ? error.message : "We could not start the subscription checkout.");
  }
});

updateSummary();
selectMode("merchant");
