package repository

import (
	"context"

	"github.com/fullwa/fullwa/internal/domain/integration"
)

// WebhookEventRepo persists raw webhook deliveries for idempotency + audit.
type WebhookEventRepo interface {
	// Insert stores a WebhookEvent. Returns created=true on a real
	// insert. On UNIQUE(integration_id, external_event_id) collisions
	// implementations return created=false with a nil error so callers
	// can absorb duplicate deliveries as no-ops.
	Insert(ctx context.Context, evt integration.WebhookEvent) (created bool, err error)

	// MarkProcessed transitions the row to Status=processed and stamps
	// ProcessedAt=now.
	MarkProcessed(ctx context.Context, id integration.WebhookEventID) error

	// MarkFailed transitions the row to Status=failed with the given
	// error message. ProcessedAt is stamped to now.
	MarkFailed(ctx context.Context, id integration.WebhookEventID, errMsg string) error

	// Get fetches a WebhookEvent by ID.
	Get(ctx context.Context, id integration.WebhookEventID) (integration.WebhookEvent, error)

	// ListPending returns up to `limit` rows still in status='received',
	// oldest first. Used by the reconciler that picks up webhooks the
	// primary consumer missed.
	ListPending(ctx context.Context, limit int) ([]integration.WebhookEvent, error)
}
