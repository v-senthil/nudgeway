// Package session models the communication relationship between one
// business endpoint (e.g. a WhatsApp Business phone number) and one
// end-user identity. A Contact may hold multiple Sessions across different
// business endpoints; at most one ACTIVE Session exists per
// (org, business_endpoint, contact).
package session

import (
	"errors"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID identifies a Session row.
type ID string

// BusinessEndpointID references a channel-specific inbound/outbound address
// owned by the tenant (e.g. a WhatsApp phone_number_id).
type BusinessEndpointID string

// Status enumerates Session lifecycle states.
type Status string

// Status values.
const (
	// StatusActive means the session is the current communication context.
	// Exactly one active session per (org, endpoint, contact) is enforced
	// via a UNIQUE index at the storage layer.
	StatusActive Status = "active"
	// StatusClosed means the session is no longer the current context.
	StatusClosed Status = "closed"
)

// Session is the persistent communication relationship between a business
// endpoint and a contact. Conversations are nested under a Session.
type Session struct {
	ID                 ID
	OrgID              organization.ID
	ContactID          contact.ID
	BusinessEndpointID BusinessEndpointID
	Status             Status
	OpenedAt           time.Time
	ClosedAt           *time.Time
	Metadata           map[string]any
}

// ErrAlreadyClosed is returned when Close is called on a non-active session.
var ErrAlreadyClosed = errors.New("session: already closed")

// ErrAlreadyActive is returned when Reopen is called on an active session.
var ErrAlreadyActive = errors.New("session: already active")

// Close marks the session closed. Idempotent errors are surfaced so callers
// can distinguish no-op writes from real transitions.
func (s *Session) Close(now time.Time) error {
	if s.Status == StatusClosed {
		return ErrAlreadyClosed
	}
	t := now.UTC()
	s.Status = StatusClosed
	s.ClosedAt = &t
	return nil
}

// Reopen flips a closed session back to active. Callers must first ensure no
// other active session exists for the (org, endpoint, contact) triple.
func (s *Session) Reopen(now time.Time) error {
	if s.Status == StatusActive {
		return ErrAlreadyActive
	}
	s.Status = StatusActive
	s.ClosedAt = nil
	s.OpenedAt = now.UTC()
	return nil
}

// IsActive is a convenience predicate.
func (s Session) IsActive() bool { return s.Status == StatusActive }
