package repository

import (
	"context"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/conversation"
	"github.com/v-senthil/nudgeway/internal/domain/group"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
	"github.com/v-senthil/nudgeway/internal/domain/user"
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

	// EnsureGroupConversation returns the Conversation row for the given
	// group, creating a fresh Type=group row if none exists yet. Idempotent:
	// a repeated call with the same (orgID, groupID) returns the existing
	// row. Implementations enforce uniqueness via a UNIQUE (org_id,
	// group_id) index on the conversations table.
	EnsureGroupConversation(ctx context.Context, orgID organization.ID, groupID group.ID) (conversation.Conversation, error)
}
