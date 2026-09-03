package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
	"github.com/fullwa/fullwa/internal/ports/queue"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// MaxWebhookBodyBytes caps how much we will read from an inbound webhook
// request. Meta batches multiple entries but stays well below 1 MiB in
// practice; anything larger is either a bug, a probe, or an attack and gets
// rejected with 413.
const MaxWebhookBodyBytes int64 = 1 << 20 // 1 MiB

// WebhookProcessLane is the queue lane that carries received deliveries to
// the async webhook worker.
const WebhookProcessLane = "webhook.process"

// Secrets exposes decrypted integration secrets to the ingress layer. The
// concrete infra implementation (mysql.Integrations) exposes a
// GetWithSecrets method — the ingress needs both Get and GetWithSecrets, so
// it takes an interface wider than the pure repository.IntegrationRepo
// port.
type Secrets interface {
	repository.IntegrationRepo

	// GetWithSecrets returns the Integration alongside a map of decrypted
	// secret material keyed by name ("access_token", "app_secret",
	// "verify_token" for WhatsApp).
	GetWithSecrets(ctx context.Context, orgID organization.ID, id integration.ID) (integration.Integration, map[string]string, error)
}

// Ingress is the provider-agnostic webhook HTTP handler. It reads the raw
// body once (capped at MaxWebhookBodyBytes), looks up the target
// integration, asks the matching provider adapter to verify the signature,
// persists a WebhookEvent row for idempotency + replay, enqueues the raw
// body onto the webhook.process lane, and returns 200 quickly so external
// providers never retry due to our own backlog.
type Ingress struct {
	// Integrations resolves integrations and decrypts their secrets on
	// demand. Multi-tenancy is enforced by the (org_id, id) match key in
	// every method.
	Integrations Secrets

	// Events persists a row per delivery. Insert is UNIQUE on
	// (integration_id, external_event_id); duplicates return created=false
	// with a nil error so we can absorb replays as no-ops.
	Events repository.WebhookEventRepo

	// Enqueuer places the raw body onto the async processing lane. The
	// ingress never processes payloads inline.
	Enqueuer queue.Enqueuer

	// Verifiers maps provider keys to their signature verifier. Populated
	// at wire-up time so the ingress does not import any provider package.
	Verifiers VerifierLookup

	// Logger receives one structured record per delivery with
	// request_id, provider, integration_id, event_id, sig_ok, dup.
	Logger *slog.Logger

	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time
}

// jobPayload is the concrete envelope written to the queue for the webhook
// worker to consume. It is JSON so the worker (in a peer package) does not
// need to import this package's types.
type jobPayload struct {
	Provider        string    `json:"provider"`
	IntegrationID   string    `json:"integration_id"`
	OrgID           string    `json:"org_id"`
	WebhookEventID  string    `json:"webhook_event_id"`
	ExternalEventID string    `json:"external_event_id"`
	Headers         [][2]string `json:"headers"`
	RawBody         []byte    `json:"raw_body"`
	ReceivedAt      time.Time `json:"received_at"`
	RequestID       string    `json:"request_id"`
}

// Handle drives one delivery end-to-end. Callers (the REST router) pass in
// the URL-derived providerKey and integrationID; nothing else about the
// request is trusted — org_id is derived from the Integration row.
//
//nolint:gocyclo,funlen // sequenced pipeline; splitting hurts readability.
func (in *Ingress) Handle(w http.ResponseWriter, r *http.Request, providerKey, integrationID string) {
	ctx := r.Context()
	reqID := middleware.RequestIDFrom(ctx)
	log := in.logger().With(
		slog.String("request_id", reqID),
		slog.String("provider", providerKey),
		slog.String("integration_id", integrationID),
	)

	// 1. Read raw body, capped.
	r.Body = http.MaxBytesReader(w, r.Body, MaxWebhookBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "payload_too_large",
				fmt.Sprintf("webhook body exceeds %d bytes", MaxWebhookBodyBytes))
			log.Warn("webhook rejected: body too large", slog.Any("err", err))
			return
		}
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "cannot read request body")
		log.Warn("webhook rejected: body read failed", slog.Any("err", err))
		return
	}

	// 2. Look up integration + secrets. org_id is derived from the row,
	// never trusted from the URL.
	integ, secrets, err := in.Integrations.GetWithSecrets(ctx, "", integration.ID(integrationID))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "integration_not_found", "unknown integration")
		log.Warn("webhook rejected: integration lookup failed", slog.Any("err", err))
		return
	}
	if integ.Provider != providerKey {
		// Prevents an attacker from posting a WhatsApp-signed body to a
		// Twilio integration id (or vice-versa).
		writeProblem(w, r, http.StatusNotFound, "integration_not_found", "provider mismatch")
		log.Warn("webhook rejected: provider mismatch",
			slog.String("row_provider", integ.Provider),
		)
		return
	}
	log = log.With(slog.String("org_id", string(integ.OrgID)))

	// 3. Signature verification via the provider adapter. This MUST run
	// before any parsing so a forged body never reaches the parser.
	verifier, err := in.Verifiers(providerKey)
	if err != nil {
		writeProblem(w, r, http.StatusUnauthorized, "signature_verification_failed",
			"no signature verifier for provider")
		log.Warn("webhook rejected: no verifier", slog.Any("err", err))
		return
	}
	appSecret := secrets["app_secret"]
	if err := verifier.VerifySignature(r.Header, raw, appSecret); err != nil {
		writeProblem(w, r, http.StatusUnauthorized, "signature_verification_failed",
			"signature verification failed")
		log.Warn("webhook rejected: signature",
			slog.Bool("sig_ok", false),
			slog.Any("err", err),
		)
		return
	}

	// 4. Extract a stable external event id.
	externalID := extractExternalEventID(providerKey, raw)

	// 5. Persist the WebhookEvent row. Duplicate = 200 (idempotent replay).
	now := in.now()
	evt := integration.WebhookEvent{
		OrgID:           integ.OrgID,
		IntegrationID:   integ.ID,
		Provider:        providerKey,
		ExternalEventID: externalID,
		ReceivedAt:      now,
		Status:          integration.WebhookStatusReceived,
		RawBody:         raw,
	}
	created, err := in.Events.Insert(ctx, evt)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "internal", "persist webhook event")
		log.Error("webhook: persist failed", slog.Any("err", err))
		return
	}
	if !created {
		// Already processed — from Meta's POV this is success.
		log.Info("webhook duplicate",
			slog.String("event_id", externalID),
			slog.Bool("sig_ok", true),
			slog.Bool("dup", true),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	// 6. Enqueue for async processing. Headers are copied so the worker can
	// re-run parsers that consume them.
	payload := jobPayload{
		Provider:        providerKey,
		IntegrationID:   string(integ.ID),
		OrgID:           string(integ.OrgID),
		WebhookEventID:  string(evt.ID),
		ExternalEventID: externalID,
		Headers:         flattenHeaders(r.Header),
		RawBody:         raw,
		ReceivedAt:      now,
		RequestID:       reqID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "internal", "encode job")
		log.Error("webhook: encode job failed", slog.Any("err", err))
		return
	}
	jobID, err := in.Enqueuer.Enqueue(ctx, queue.Job{
		Lane:    WebhookProcessLane,
		Payload: body,
	})
	if err != nil {
		// Row is persisted; a reconciler pass over Status=received rows
		// will pick this up. Still ACK 200 so Meta doesn't retry — the
		// authoritative state is in webhook_events already.
		log.Error("webhook: enqueue failed (row persisted; reconciler will pick up)",
			slog.Any("err", err),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	// 7. ACK 200 (empty body).
	log.Info("webhook accepted",
		slog.String("event_id", externalID),
		slog.String("job_id", jobID),
		slog.Bool("sig_ok", true),
		slog.Bool("dup", false),
	)
	w.WriteHeader(http.StatusOK)
}

// VerifyHandshake implements the GET verification handshake used by Meta
// and other providers that follow the same pattern (hub.mode / hub.challenge
// / hub.verify_token). It is exposed as a separate method so the REST layer
// can wire it without duplicating integration lookup + secret decryption
// logic.
//
// Returns 200 text/plain with hub.challenge on a matching verify token, and
// 403 problem+json otherwise.
func (in *Ingress) VerifyHandshake(w http.ResponseWriter, r *http.Request, providerKey, integrationID string) {
	ctx := r.Context()
	reqID := middleware.RequestIDFrom(ctx)
	log := in.logger().With(
		slog.String("request_id", reqID),
		slog.String("provider", providerKey),
		slog.String("integration_id", integrationID),
	)

	q := r.URL.Query()
	mode := q.Get("hub.mode")
	token := q.Get("hub.verify_token")
	challenge := q.Get("hub.challenge")
	if mode != "subscribe" {
		writeProblem(w, r, http.StatusForbidden, "verify_failed", "hub.mode must be subscribe")
		log.Warn("verify handshake rejected: bad mode", slog.String("mode", mode))
		return
	}

	integ, secrets, err := in.Integrations.GetWithSecrets(ctx, "", integration.ID(integrationID))
	if err != nil {
		writeProblem(w, r, http.StatusForbidden, "verify_failed", "unknown integration")
		log.Warn("verify handshake rejected: integration lookup failed", slog.Any("err", err))
		return
	}
	if integ.Provider != providerKey {
		writeProblem(w, r, http.StatusForbidden, "verify_failed", "provider mismatch")
		return
	}
	want := secrets["verify_token"]
	if want == "" || token != want {
		writeProblem(w, r, http.StatusForbidden, "verify_failed", "verify token mismatch")
		log.Warn("verify handshake rejected: token mismatch",
			slog.String("org_id", string(integ.OrgID)),
		)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)
	log.Info("verify handshake ok",
		slog.String("org_id", string(integ.OrgID)),
	)
}

// extractExternalEventID picks the most stable identifier we can compute
// from the raw body. Meta webhook envelopes don't ship a top-level event id
// on the container, so we fall back to sha256(raw_body). Because the
// WebhookEvent row is unique on (integration_id, external_event_id), this
// is enough to dedupe genuine replays (Meta re-delivers the exact same
// bytes on retry).
func extractExternalEventID(_ /*providerKey*/ string, raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// flattenHeaders converts http.Header into a slice pair form suitable for
// JSON encoding without losing multi-value semantics.
func flattenHeaders(h http.Header) [][2]string {
	out := make([][2]string, 0, len(h))
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

func (in *Ingress) now() time.Time {
	if in.Now != nil {
		return in.Now().UTC()
	}
	return time.Now().UTC()
}

func (in *Ingress) logger() *slog.Logger {
	if in.Logger != nil {
		return in.Logger
	}
	return slog.Default()
}

// writeProblem writes an RFC 7807 problem+json response. Duplicated from
// the REST v1 package on purpose so the webhook layer has zero dependency
// on that package.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "https://fullwa.dev/errors/" + title,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"request_id": middleware.RequestIDFrom(r.Context()),
	})
}
