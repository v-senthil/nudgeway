package repository

import (
	"context"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/identity"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// IdentityRepo persists ContactIdentity rows.
type IdentityRepo interface {
	// FindOrCreate looks up an identity by (org, provider, normalizedValue).
	// If missing, it creates one bound to a Contact allocated by the caller
	// (contactID); created reports whether an insert happened.
	//
	// Implementations MUST perform this atomically (INSERT ... ON DUPLICATE
	// KEY UPDATE or equivalent) — no read-then-write race windows.
	FindOrCreate(
		ctx context.Context,
		orgID organization.ID,
		contactID contact.ID,
		identityType identity.Type,
		provider string,
		value string,
		normalizedValue string,
	) (id identity.Identity, created bool, err error)

	// Get fetches an identity by (OrgID, ID).
	Get(ctx context.Context, orgID organization.ID, id identity.ID) (identity.Identity, error)

	// ListForContact returns all identities bound to a Contact.
	ListForContact(ctx context.Context, orgID organization.ID, contactID contact.ID) ([]identity.Identity, error)
}
