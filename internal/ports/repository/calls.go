package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/call"
	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/conversation"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// CallListFilter narrows a CallRepo.List query. All fields are optional;
// zero values disable the corresponding predicate. Limit defaults to 50 and
// is capped at 200.
type CallListFilter struct {
	// Direction restricts to inbound / outbound.
	Direction call.Direction
	// Status restricts to a single status value.
	Status call.Status
	// ContactID restricts to calls involving a specific contact. Requires
	// the composite (org, contact_id, created_at) index.
	ContactID *contact.ID
	// ConversationID restricts to calls attached to a specific
	// conversation. Requires the composite (org, conversation_id,
	// created_at) index.
	ConversationID *conversation.ID
	// Since restricts to CreatedAt >= Since.
	Since time.Time
	// Until restricts to CreatedAt < Until.
	Until time.Time
	// Limit caps the returned page size. Zero picks 50.
	Limit int
	// Cursor is the opaque continuation token returned by the previous
	// page. Empty starts from the newest entry.
	Cursor string
}

// CallPage is a page of Call rows ordered newest-first.
type CallPage struct {
	Items      []call.Call
	NextCursor string
}

// CallRepo persists Call rows. Every method takes an OrgID and enforces
// tenancy in every query.
type CallRepo interface {
	// Create inserts a new Call row. Callers pre-populate the ID.
	// Implementations enforce uniqueness on (org_id, provider,
	// provider_call_id).
	Create(ctx context.Context, c call.Call) error

	// UpsertByProviderID inserts the row when absent, or updates the
	// existing row when the (org_id, provider, provider_call_id) tuple
	// already exists. Used by webhook ingest so duplicate deliveries are
	// idempotent.
	//
	// Implementations preserve any timestamps already stamped in the
	// existing row (started_at, answered_at, ended_at) when the incoming
	// row leaves them zero — a later status callback should not blank a
	// prior stamped answer.
	UpsertByProviderID(ctx context.Context, c call.Call) (call.Call, error)

	// Get fetches a single Call row by (OrgID, ID). Returns call.ErrNotFound
	// when the row does not exist.
	Get(ctx context.Context, orgID organization.ID, id call.ID) (call.Call, error)

	// List returns a page of Call rows newest-first for org.
	List(ctx context.Context, orgID organization.ID, filter CallListFilter) (CallPage, error)

	// UpdateStatus advances a call's Status + stamps the appropriate
	// timestamp (started_at when Status=ringing, answered_at when
	// Status=answered, ended_at when the status is terminal). Idempotent
	// when the current row is already at a terminal status.
	UpdateStatus(ctx context.Context, orgID organization.ID, id call.ID, next call.Status, at time.Time) error

	// AttachRecording stamps recording_url + duration_seconds onto an
	// existing Call. Used by the recording webhook consumer.
	AttachRecording(ctx context.Context, orgID organization.ID, id call.ID, recordingURL string, durationSeconds int) error
}
