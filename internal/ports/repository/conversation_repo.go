package repository

import (
	"context"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/session"
	"github.com/fullwa/fullwa/internal/domain/user"
)

// ConversationListFilter narrows a Conversation listing.
type ConversationListFilter struct {
	Cursor string
	Limit  int
	Status *conversation.Status
}

// ConversationPage is a page of conversations.
type ConversationPage struct {
	Conversations []conversation.Conversation
	NextCursor    string
}

// ConversationRepo persists Conversation rows.
type ConversationRepo interface {
	// FindOrCreateOpen returns the current OPEN/REOPENED conversation for
	// the session, opening a new one if none exists.
	FindOrCreateOpen(ctx context.Context, orgID organization.ID, sessionID session.ID, contactID contact.ID) (conversation.Conversation, error)

	// Get fetches by (OrgID, ID).
	Get(ctx context.Context, orgID organization.ID, id conversation.ID) (conversation.Conversation, error)

	// UpdateStatus persists a Status transition, along with derived
	// timestamps (ResolvedAt, LastMessageAt).
	UpdateStatus(ctx context.Context, orgID organization.ID, id conversation.ID, status conversation.Status) error

	// Assign persists an assignment change. Either or both may be nil to
	// clear the corresponding assignee.
	Assign(ctx context.Context, orgID organization.ID, id conversation.ID, userID *user.ID, teamID *conversation.TeamID) error

	// ListForContact returns conversations for a contact, newest first.
	ListForContact(ctx context.Context, orgID organization.ID, contactID contact.ID, filter ConversationListFilter) (ConversationPage, error)
}
