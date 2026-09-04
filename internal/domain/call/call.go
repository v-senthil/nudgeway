package call

import (
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/conversation"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
)

// ID identifies a canonical Call row (ULID string form).
type ID string

// Direction distinguishes calls placed by the business from calls placed
// by the customer.
type Direction string

// Direction values.
const (
	// DirectionInbound is a user-initiated call — the customer called the
	// business number.
	DirectionInbound Direction = "inbound"
	// DirectionOutbound is a business-initiated call.
	DirectionOutbound Direction = "outbound"
)

// Status enumerates the call's position in the canonical state machine.
// Provider-specific status vocabularies are mapped onto these by the
// adapter — the domain layer never sees a provider status string.
type Status string

// Status values. The state machine allows the following transitions:
//
//	queued      → ringing | failed
//	ringing     → answered | missed | declined | no_answer | failed
//	answered    → in_progress | completed | failed
//	in_progress → completed | failed
//	terminal: completed | missed | failed | declined | no_answer
//
// UpsertByProviderID accepts any status; the domain validation runs on
// caller code that computes the next status from a webhook event.
const (
	StatusQueued     Status = "queued"
	StatusRinging    Status = "ringing"
	StatusAnswered   Status = "answered"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusMissed     Status = "missed"
	StatusFailed     Status = "failed"
	StatusDeclined   Status = "declined"
	StatusNoAnswer   Status = "no_answer"
)

// Terminal reports whether s is a terminal status (no further transitions).
func (s Status) Terminal() bool {
	switch s {
	case StatusCompleted, StatusMissed, StatusFailed, StatusDeclined, StatusNoAnswer:
		return true
	}
	return false
}

// Call is the canonical call entity. Provider-agnostic. Rows are persisted
// by the CallRepo; the source of truth for state is this table, not the
// provider — the provider surfaces events which the application layer
// translates onto Status transitions.
type Call struct {
	// ID is the canonical ULID.
	ID ID
	// OrgID is the tenant boundary.
	OrgID organization.ID
	// IntegrationID identifies the integration that owns the call.
	IntegrationID string
	// BusinessEndpointID is the endpoint (phone number) the call was placed
	// through. Nil when we couldn't resolve one (e.g. an inbound webhook
	// for an endpoint that has been deleted).
	BusinessEndpointID *session.BusinessEndpointID
	// ContactID links the call to the customer. Nil until the ingest
	// pipeline upserts a contact.
	ContactID *contact.ID
	// SessionID links the call to the customer's active comms session.
	SessionID *session.ID
	// ConversationID stitches the call onto the customer's conversation
	// history. Nil until reconciliation runs.
	ConversationID *conversation.ID
	// Provider is the registry key of the adapter ("whatsapp", ...).
	Provider string
	// ProviderCallID is the provider-native call id (e.g. wacid.*). Unique
	// per (org, provider).
	ProviderCallID string
	// Direction distinguishes user-initiated from business-initiated calls.
	Direction Direction
	// Status is the current state.
	Status Status
	// From is the caller identity (E.164 phone or BSUID for inbound).
	From string
	// To is the callee identity (E.164 phone or BSUID for outbound).
	To string
	// FromUserID is the caller's BSUID when known.
	FromUserID string
	// ToUserID is the callee's BSUID when known.
	ToUserID string
	// StartedAt is when the call was placed (queued → ringing).
	StartedAt *time.Time
	// AnsweredAt is when the call was picked up.
	AnsweredAt *time.Time
	// EndedAt is when the call ended.
	EndedAt *time.Time
	// DurationSeconds is the total wall-clock duration in seconds. Zero
	// when the call never connected.
	DurationSeconds int
	// HangupReason is a short provider-specific hangup code / reason.
	HangupReason string
	// RecordingURL points at the recording. Format depends on the recording
	// pipeline: provider short-lived URL, or /api/v1/media/<key> once the
	// downloader lands.
	RecordingURL string
	// TranscriptionRef is the provider media id for the transcript JSON.
	TranscriptionRef string
	// Extras carries free-form provider-specific fields. Persisted as
	// a JSON column by the repository layer.
	Extras map[string]any
	// CreatedAt is when the row was inserted.
	CreatedAt time.Time
	// UpdatedAt is when the row was last mutated.
	UpdatedAt *time.Time
}

// ApplyEvent advances the Call in place per an event kind. The mapping is
// intentionally lenient — an out-of-order webhook (e.g. terminate arriving
// before status callbacks) does not corrupt state; the newer terminal
// status wins.
type EventKind string

// EventKind values map to the canonical events emitted by the provider
// webhook parser. Adapters translate their vocabulary onto these.
const (
	EventReceived  EventKind = "received"  // inbound call rang in
	EventRinging   EventKind = "ringing"   // outbound call reached ringing on user's device
	EventAnswered  EventKind = "answered"  // callee picked up
	EventEnded     EventKind = "ended"     // call completed cleanly
	EventFailed    EventKind = "failed"    // call failed
	EventRecording EventKind = "recording" // recording available
)
