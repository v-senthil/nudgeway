// Package conversation models the customer-service work context. A Session
// may hold multiple Conversations over its lifetime — distinct threads with
// their own status, assignment, priority, and SLA state.
package conversation

import (
	"errors"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/group"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
	"github.com/v-senthil/nudgeway/internal/domain/user"
)

// ID identifies a Conversation row.
type ID string

// TeamID identifies an assignable agent team.
type TeamID string

// Type distinguishes a 1-to-1 conversation from a group conversation. The
// two share the same lifecycle + assignment semantics; they differ only in
// which foreign key column is populated (contact_id vs group_id) and in how
// the inbox UI labels + composes messages against them.
type Type string

// Type values.
const (
	// TypeOneToOne is the default: a thread bound to one Contact through a
	// Session. Both ContactID and SessionID are populated.
	TypeOneToOne Type = "one_to_one"

	// TypeGroup is a group thread: bound to a Group row via GroupID.
	// ContactID and SessionID are empty on group-typed conversations.
	TypeGroup Type = "group"
)

// Status enumerates Conversation lifecycle states.
type Status string

// Status values.
const (
	StatusOpen     Status = "open"     // active work-in-progress
	StatusPending  Status = "pending"  // awaiting customer / third party
	StatusResolved Status = "resolved" // closed successfully
	StatusReopened Status = "reopened" // previously resolved, reopened
)

// Priority enumerates business urgency.
type Priority string

// Priority values.
const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// Conversation is a customer-service thread. Historically 1-to-1 (contact +
// session); Type=group threads carry a GroupID instead and leave SessionID /
// ContactID empty.
//
// SessionID + ContactID stay as value types (empty string = absent for
// group-typed rows) to avoid a wide pointer refactor across the send /
// inbound / repo paths. Callers that need to render or query group threads
// should branch on Type.
type Conversation struct {
	ID             ID
	OrgID          organization.ID
	SessionID      session.ID       // empty for Type=group
	ContactID      contact.ID       // empty for Type=group
	Type           Type             // "one_to_one" (default) or "group"
	GroupID        *group.ID        // set iff Type=group
	Status         Status
	AssignedUserID *user.ID
	AssignedTeamID *TeamID
	Priority       Priority
	UnreadCount    int
	LastMessageAt  *time.Time
	SLADueAt       *time.Time
	AIState        string
	BotState       string
	Tags           []string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
}

// ErrInvalidTransition is returned for illegal Status transitions.
var ErrInvalidTransition = errors.New("conversation: invalid status transition")

// Assign sets the responsible user and/or team. Passing nil for both clears
// the assignment (unassigned).
func (c *Conversation) Assign(u *user.ID, t *TeamID) {
	c.AssignedUserID = u
	c.AssignedTeamID = t
}

// Resolve moves an open/pending/reopened conversation to resolved. Idempotent
// resolves return ErrInvalidTransition so callers can distinguish.
func (c *Conversation) Resolve(now time.Time) error {
	switch c.Status {
	case StatusOpen, StatusPending, StatusReopened:
		c.Status = StatusResolved
		t := now.UTC()
		c.ResolvedAt = &t
		return nil
	default:
		return ErrInvalidTransition
	}
}

// Reopen moves a resolved conversation back into the working set.
func (c *Conversation) Reopen(now time.Time) error {
	if c.Status != StatusResolved {
		return ErrInvalidTransition
	}
	c.Status = StatusReopened
	c.ResolvedAt = nil
	c.LastMessageAt = ptrTime(now.UTC())
	return nil
}

// MarkRead zeroes the unread counter (e.g. when an agent opens the thread).
func (c *Conversation) MarkRead() { c.UnreadCount = 0 }

// RecordInbound bumps the unread counter and advances LastMessageAt.
func (c *Conversation) RecordInbound(at time.Time) {
	c.UnreadCount++
	t := at.UTC()
	c.LastMessageAt = &t
}

// RecordOutbound only advances LastMessageAt.
func (c *Conversation) RecordOutbound(at time.Time) {
	t := at.UTC()
	c.LastMessageAt = &t
}

func ptrTime(t time.Time) *time.Time { return &t }
