package repository

import (
	"context"
	"time"

	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
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

	// UpdateStatus advances the message's Status + timestamps. When a
	// terminal status has already been recorded, implementations return
	// nil (idempotent no-op) rather than an error.
	UpdateStatus(ctx context.Context, orgID organization.ID, id message.ID, next message.Status, at time.Time) error

	// ListByConversation returns metadata rows for a conversation newest
	// first. Payloads are fetched separately by conversation view code.
	ListByConversation(ctx context.Context, orgID organization.ID, convID conversation.ID, filter MessageListFilter) (MessagePage, error)
}
