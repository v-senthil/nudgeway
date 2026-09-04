package v1

import (
	"net/http"

	"github.com/v-senthil/nudgeway/internal/webhook"
)

// WebhookDeps bundles what the webhook routes need. The ingress helper owns
// the actual pipeline; this package just installs the URL patterns.
type WebhookDeps struct {
	// Ingress is the fully-wired ingress pipeline (integration lookup,
	// signature verification, persistence, enqueue). Provided by
	// cmd/server.
	Ingress *webhook.Ingress
}

// mountWebhooks installs the provider-agnostic webhook endpoints on mux.
// The endpoints live OUTSIDE /api/v1/* because external providers (Meta,
// Twilio, ...) do not carry our session cookie or CSRF token — they are
// unauthenticated at the HTTP layer and rely on the per-provider signature
// verification for authenticity. Middleware chain here is deliberately
// minimal: RequestID → Recover → Logger only.
func mountWebhooks(mux Registrar, base func(http.Handler) http.Handler, deps WebhookDeps) {
	if deps.Ingress == nil {
		return
	}
	h := &webhookHandler{ing: deps.Ingress}

	mux.Handle("GET /webhooks/{provider}/{integration_id}", base(http.HandlerFunc(h.verify)))
	mux.Handle("POST /webhooks/{provider}/{integration_id}", base(http.HandlerFunc(h.receive)))
}

// webhookHandler bundles state for the two ingress endpoints so we can hang
// the receive + verify methods off a single value.
type webhookHandler struct {
	ing *webhook.Ingress
}

// verify handles Meta's subscription verification GET. It validates
// hub.mode / hub.verify_token / hub.challenge against the target
// integration's stored verify_token secret.
func (h *webhookHandler) verify(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	integrationID := r.PathValue("integration_id")
	h.ing.VerifyHandshake(w, r, provider, integrationID)
}

// receive handles the actual signed webhook delivery.
func (h *webhookHandler) receive(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	integrationID := r.PathValue("integration_id")
	h.ing.Handle(w, r, provider, integrationID)
}
