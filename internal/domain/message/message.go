// Package message models canonical messages. Every Message belongs to a
// Session and a Conversation, and carries provider-agnostic status, type,
// and metadata. Message payloads live in HBase, referenced by payload_ref.
package message

import (
	"errors"
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/session"
)

// ID identifies a Message row.
type ID string

// Direction describes whether a message was received from or sent to a customer.
type Direction string

// Direction values.
const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

// Type is the provider-neutral message category. Adapters map their native
// message types onto this vocabulary (see internal/providers/whatsapp/mapper.go).
type Type string

// Type values.
const (
	TypeText        Type = "text"
	TypeImage       Type = "image"
	TypeVideo       Type = "video"
	TypeAudio       Type = "audio"
	TypeDocument    Type = "document"
	TypeSticker     Type = "sticker"
	TypeLocation    Type = "location"
	TypeContacts    Type = "contacts"
	TypeTemplate    Type = "template"
	TypeInteractive Type = "interactive"
	TypeReaction    Type = "reaction"
	TypeButton      Type = "button"  // template quick-reply tap
	TypeSystem      Type = "system"
	TypeUnknown     Type = "unknown" // preserve raw payload in Metadata
)

// Status is the canonical delivery lifecycle state.
type Status string

// Status values. The state machine is enforced by Transition().
const (
	StatusQueued    Status = "queued"
	StatusSent      Status = "sent"
	StatusDelivered Status = "delivered"
	StatusRead      Status = "read"
	StatusFailed    Status = "failed"
)

// Message is the metadata row for a message. The full payload lives in HBase
// referenced by PayloadRef.
type Message struct {
	ID                ID
	OrgID             organization.ID
	ContactID         contact.ID
	SessionID         session.ID
	ConversationID    conversation.ID
	Channel           string // "whatsapp", "email", ...
	Provider          string // "whatsapp", "twilio", ...
	Direction         Direction
	SenderIdentity    string // canonical (E.164, email, wa_id-with-plus)
	RecipientIdentity string
	MessageType       Type
	ProviderMessageID string // e.g. wamid — used for idempotent status updates
	Status            Status
	CreatedAt         time.Time
	SentAt            *time.Time
	DeliveredAt       *time.Time
	ReadAt            *time.Time
	PayloadRef        string         // HBase row key or opaque ref
	Metadata          map[string]any // provider-specific bag (kept out of the hot path)
}

// ErrInvalidStatusTransition rejects illegal state changes.
var ErrInvalidStatusTransition = errors.New("message: invalid status transition")

// valid holds the state-machine adjacency. QUEUED→SENT→DELIVERED→READ,
// with QUEUED→FAILED and SENT→FAILED as terminal error edges. Inbound
// messages skip queued and land as delivered; that's set at construction,
// not via Transition().
var valid = map[Status]map[Status]bool{
	StatusQueued: {
		StatusSent:   true,
		StatusFailed: true,
	},
	StatusSent: {
		StatusDelivered: true,
		StatusRead:      true, // Meta may collapse delivered→read
		StatusFailed:    true,
	},
	StatusDelivered: {
		StatusRead:   true,
		StatusFailed: true, // rare, but Meta can post-fail
	},
	StatusRead:   {},
	StatusFailed: {},
}

// Transition applies a Status change if the state machine allows it and
// stamps the corresponding timestamp field. Callers pass the timestamp
// reported by the provider (or time.Now for locally-generated transitions).
func (m *Message) Transition(next Status, at time.Time) error {
	if m.Status == next {
		return nil // idempotent no-op
	}
	edges, ok := valid[m.Status]
	if !ok || !edges[next] {
		return ErrInvalidStatusTransition
	}
	t := at.UTC()
	switch next {
	case StatusSent:
		m.SentAt = &t
	case StatusDelivered:
		m.DeliveredAt = &t
		if m.SentAt == nil {
			m.SentAt = &t
		}
	case StatusRead:
		m.ReadAt = &t
		if m.DeliveredAt == nil {
			m.DeliveredAt = &t
		}
		if m.SentAt == nil {
			m.SentAt = &t
		}
	}
	m.Status = next
	return nil
}

// Terminal reports whether the status is a terminal one.
func (m Message) Terminal() bool {
	return m.Status == StatusRead || m.Status == StatusFailed
}
