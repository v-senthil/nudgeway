package integration

import (
	"time"

	"github.com/fullwa/fullwa/internal/domain/organization"
)

// WebhookEventID identifies a persisted webhook_events row.
type WebhookEventID string

// WebhookEventStatus enumerates the ingestion lifecycle for a single
// webhook delivery.
type WebhookEventStatus string

// WebhookEventStatus values.
const (
	// WebhookStatusReceived: raw envelope persisted, not yet picked up.
	WebhookStatusReceived WebhookEventStatus = "received"
	// WebhookStatusProcessing: a worker has claimed the row.
	WebhookStatusProcessing WebhookEventStatus = "processing"
	// WebhookStatusProcessed: parsing + downstream persistence succeeded.
	WebhookStatusProcessed WebhookEventStatus = "processed"
	// WebhookStatusFailed: unrecoverable parse/persist error.
	WebhookStatusFailed WebhookEventStatus = "failed"
)

// WebhookEvent is a raw provider webhook delivery persisted for
// idempotency + audit. The (IntegrationID, ExternalEventID) tuple is
// UNIQUE at the storage layer; duplicate deliveries are absorbed as
// no-ops. RawBody carries the exact bytes we ACKed against for later
// replay + debugging.
type WebhookEvent struct {
	ID              WebhookEventID
	OrgID           organization.ID
	IntegrationID   ID
	Provider        string
	ExternalEventID string
	ReceivedAt      time.Time
	ProcessedAt     *time.Time
	Status          WebhookEventStatus
	RawBody         []byte
	Error           string
}
