package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	appmsg "github.com/fullwa/fullwa/internal/application/message"
	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/ports/queue"
)

// WebhookLane is the queue-lane name for asynchronous webhook processing.
// Ingress enqueues onto this lane; the worker consumes from it.
const WebhookLane = "webhook.process"

// WebhookJobPayload is the JSON body enqueued by the ingress handler for
// each accepted webhook delivery. Keep it small — the raw body is the
// bulk of the bytes.
type WebhookJobPayload struct {
	// Provider is the provider-registry key of the adapter that must
	// parse the body ("whatsapp", "twilio", ...).
	Provider string `json:"provider"`
	// IntegrationID identifies the tenant-scoped provider configuration.
	IntegrationID string `json:"integration_id"`
	// EventID is the WebhookEvent row id created by the ingress layer.
	// Empty is tolerated (direct invocation / smoke paths).
	EventID string `json:"event_id,omitempty"`
	// RawBody is the exact bytes signature-verified at ingress. The
	// worker forwards them to the adapter unchanged — re-serialising
	// would break MAC parity for downstream replays.
	RawBody []byte `json:"raw_body"`
}

// WebhookWorker consumes the webhook.process lane and drives the
// InboundService for each job. The worker owns no goroutines directly —
// spawn concurrency via workers.Pool.
type WebhookWorker struct {
	// Inbound is the application service that turns a raw webhook body
	// into persisted domain objects + published events.
	Inbound *appmsg.InboundService
	// Log receives structured errors + per-job traces. Required — the
	// worker refuses to run without one to avoid silent drops.
	Log *slog.Logger
}

// NewWebhookWorker constructs a WebhookWorker. Panics on nil dependencies
// — this is a wiring-time check, not a runtime error.
func NewWebhookWorker(inbound *appmsg.InboundService, log *slog.Logger) *WebhookWorker {
	if inbound == nil {
		panic("workers: NewWebhookWorker: inbound service is required")
	}
	if log == nil {
		panic("workers: NewWebhookWorker: logger is required")
	}
	return &WebhookWorker{Inbound: inbound, Log: log}
}

// Run subscribes the worker's handler to WebhookLane on the given
// consumer group and blocks until ctx is cancelled or the consumer
// returns an error. Callers wrap this in a bounded pool via workers.Pool.
func (w *WebhookWorker) Run(ctx context.Context, consumer queue.Consumer, group string) error {
	if consumer == nil {
		return fmt.Errorf("workers: WebhookWorker.Run: consumer is required")
	}
	if group == "" {
		return fmt.Errorf("workers: WebhookWorker.Run: consumer group is required")
	}
	return consumer.Consume(ctx, WebhookLane, group, w.handle)
}

// handle decodes one queue.Job and hands it to InboundService.ProcessRaw.
// Return value semantics follow queue.Consumer contract:
//   - nil  → ack the job (permanent errors already logged + marked failed).
//   - err  → nack for retry.
func (w *WebhookWorker) handle(ctx context.Context, job queue.Job) error {
	var payload WebhookJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		w.Log.ErrorContext(ctx, "webhook worker: malformed job payload",
			slog.String("job_id", job.ID),
			slog.String("lane", job.Lane),
			slog.Any("err", err),
		)
		// Cannot process — permanent, ack so we don't retry a broken row.
		return nil
	}
	if payload.Provider == "" || payload.IntegrationID == "" || len(payload.RawBody) == 0 {
		w.Log.ErrorContext(ctx, "webhook worker: missing required fields",
			slog.String("job_id", job.ID),
			slog.String("provider", payload.Provider),
			slog.String("integration_id", payload.IntegrationID),
			slog.Int("body_len", len(payload.RawBody)),
		)
		return nil
	}

	err := w.Inbound.ProcessRaw(
		ctx,
		payload.Provider,
		integration.ID(payload.IntegrationID),
		integration.WebhookEventID(payload.EventID),
		payload.RawBody,
	)
	if err != nil {
		// Transient — return so consumer redelivers per its backoff.
		w.Log.WarnContext(ctx, "webhook worker: transient failure, will retry",
			slog.String("job_id", job.ID),
			slog.String("provider", payload.Provider),
			slog.String("integration_id", payload.IntegrationID),
			slog.Int("attempt", job.Attempt),
			slog.Any("err", err),
		)
		return err
	}
	return nil
}
