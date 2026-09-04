package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/conversation"
	"github.com/v-senthil/nudgeway/internal/domain/message"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// MessageListFilter narrows a Message listing.
type MessageListFilter struct {
	Cursor string
	Limit  int
	// Since restricts to messages with CreatedAt >= Since; zero means unbounded.
	Since time.Time
}

// MessagePage is a page of messages ordered newest-first.
type MessagePage struct {
	Messages   []message.Message
	NextCursor string
}

// MessageRepo persists Message metadata rows. The actual payload lives in
// HBase referenced by Message.PayloadRef; that write is handled by a
// separate MessagePayloadRepo (added when HBase lands).
type MessageRepo interface {
	// Create inserts a new message row. Callers pre-populate the ID.
	// Implementations enforce uniqueness on (org, provider_message_id) so
	// duplicate webhook deliveries do not insert twice — the returned
	// error must be distinguishable as a duplicate by the caller.
	Create(ctx context.Context, m message.Message) error

	// Get returns a single message row by (org, id). Implementations return
	// a not-found error when the row does not exist. Payloads live in
	// HBase and are fetched separately.
	Get(ctx context.Context, orgID organization.ID, id message.ID) (message.Message, error)

	// UpdateStatus advances the message's Status + timestamps. When a
	// terminal status has already been recorded, implementations return
	// nil (idempotent no-op) rather than an error.
	UpdateStatus(ctx context.Context, orgID organization.ID, id message.ID, next message.Status, at time.Time) error

	// ListByConversation returns metadata rows for a conversation newest
	// first. Payloads are fetched separately by conversation view code.
	ListByConversation(ctx context.Context, orgID organization.ID, convID conversation.ID, filter MessageListFilter) (MessagePage, error)

	// FindByCallID returns the synthetic "call" message row that references
	// the given call id in its metadata (metadata.call_id). Used by the call
	// ingest pipeline to dedupe repeated webhook deliveries — a repeat call
	// event must not insert a second inline message. Implementations return
	// the infrastructure-layer not-found sentinel when no such row exists.
	FindByCallID(ctx context.Context, orgID organization.ID, callID string) (message.Message, error)

	// FindByCallIDAndStatus returns the info message row for the given
	// (call_id, call_status) tuple, used by the call ingest pipeline to
	// dedupe per-status transitions (one info row per status). The lookup
	// keys on metadata.call_id + metadata.call_status. Implementations
	// return the infrastructure-layer not-found sentinel when no such row
	// exists.
	FindByCallIDAndStatus(ctx context.Context, orgID organization.ID, callID, status string) (message.Message, error)
}
