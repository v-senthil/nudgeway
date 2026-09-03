// Package conversation models the customer-service work context. A Session
// may hold multiple Conversations over its lifetime — distinct threads with
// their own status, assignment, priority, and SLA state.
package conversation

import (
	"errors"
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/session"
	"github.com/fullwa/fullwa/internal/domain/user"
)

// ID identifies a Conversation row.
type ID string

// TeamID identifies an assignable agent team.
type TeamID string

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

// Conversation is a customer-service thread inside a Session.
type Conversation struct {
	ID              ID
	OrgID           organization.ID
	SessionID       session.ID
	ContactID       contact.ID
	Status          Status
	AssignedUserID  *user.ID
	AssignedTeamID  *TeamID
	Priority        Priority
	UnreadCount     int
	LastMessageAt   *time.Time
	SLADueAt        *time.Time
	AIState         string
	BotState        string
	Tags            []string
	CreatedAt       time.Time
	ResolvedAt      *time.Time
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
