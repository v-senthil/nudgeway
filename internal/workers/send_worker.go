package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	appmsg "github.com/v-senthil/nudgeway/internal/application/message"
	"github.com/v-senthil/nudgeway/internal/ports/queue"
)

// SendWorker consumes the outbound message send lane and drives the
// SendService.ProcessSend use-case for each job. Mirrors WebhookWorker's
// shape so the two hot paths share the same operational surface (Run,
// bounded pool via workers.Pool, structured error logging).
type SendWorker struct {
	// Send is the application service that resolves the integration,
	// invokes the provider adapter, and updates the message status.
	Send *appmsg.SendService
	// Log receives structured errors + per-job traces. Required — the
	// worker refuses to run without one to avoid silent drops.
	Log *slog.Logger
}

// NewSendWorker constructs a SendWorker. Panics on nil dependencies —
// this is a wiring-time check, not a runtime error.
func NewSendWorker(send *appmsg.SendService, log *slog.Logger) *SendWorker {
	if send == nil {
		panic("workers: NewSendWorker: send service is required")
	}
	if log == nil {
		panic("workers: NewSendWorker: logger is required")
	}
	return &SendWorker{Send: send, Log: log}
}

// Run subscribes the worker's handler to appmsg.SendLane on the given
// consumer group and blocks until ctx is cancelled or the consumer returns
// an error. Callers wrap this in a bounded pool via workers.Pool.
func (w *SendWorker) Run(ctx context.Context, consumer queue.Consumer, group string) error {
	if consumer == nil {
		return fmt.Errorf("workers: SendWorker.Run: consumer is required")
	}
	if group == "" {
		return fmt.Errorf("workers: SendWorker.Run: consumer group is required")
	}
	return consumer.Consume(ctx, appmsg.SendLane, group, w.handle)
}

// handle decodes one queue.Job and hands it to SendService.ProcessSend.
// Return value semantics follow queue.Consumer contract:
//   - nil  → ack the job (either succeeded or was marked as permanent failure).
//   - err  → nack for retry with backoff.
func (w *SendWorker) handle(ctx context.Context, job queue.Job) error {
	var payload appmsg.SendJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		w.Log.ErrorContext(ctx, "send worker: malformed job payload",
			slog.String("job_id", job.ID),
			slog.String("lane", job.Lane),
			slog.Any("err", err),
		)
		// Cannot process — permanent, ack so we don't retry a broken row.
		return nil
	}
	if payload.MessageID == "" || payload.OrgID == "" || payload.IntegrationID == "" {
		w.Log.ErrorContext(ctx, "send worker: missing required fields",
			slog.String("job_id", job.ID),
			slog.String("message_id", payload.MessageID),
			slog.String("org_id", payload.OrgID),
			slog.String("integration_id", payload.IntegrationID),
		)
		return nil
	}
	if err := w.Send.ProcessSend(ctx, payload); err != nil {
		// Transient — return so consumer redelivers per its backoff.
		w.Log.WarnContext(ctx, "send worker: transient failure, will retry",
			slog.String("job_id", job.ID),
			slog.String("message_id", payload.MessageID),
			slog.String("integration_id", payload.IntegrationID),
			slog.Int("attempt", job.Attempt),
			slog.Any("err", err),
		)
		return err
	}
	return nil
}
