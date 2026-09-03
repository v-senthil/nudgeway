package repository

import (
	"context"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// ContactListFilter is an org-scoped list filter.
type ContactListFilter struct {
	// Cursor is an opaque paging token from a prior page's NextCursor.
	Cursor string
	// Limit caps returned rows; implementations enforce a hard maximum.
	Limit int
	// Query is a case-insensitive substring match on DisplayName + primary
	// identity value; empty means no text filter.
	Query string
}

// ContactPage is a page of contacts with a cursor for the next page.
type ContactPage struct {
	Contacts   []contact.Contact
	NextCursor string
}

// ContactRepo persists Contact aggregates. All methods are org-scoped —
// implementations MUST NOT read across tenants.
type ContactRepo interface {
	// Upsert inserts or updates a Contact identified by (OrgID, ID). When
	// the row does not exist it is inserted with CreatedAt=now.
	Upsert(ctx context.Context, c contact.Contact) error

	// Get fetches a Contact by (OrgID, ID). Returns a sentinel not-found
	// error from the infrastructure layer (ErrNotFound) when missing.
	Get(ctx context.Context, orgID organization.ID, id contact.ID) (contact.Contact, error)

	// FindByPrimaryIdentity returns the contact whose PrimaryIdentityID
	// points at the given identity, if any.
	FindByPrimaryIdentity(ctx context.Context, orgID organization.ID, identityID contact.IdentityID) (contact.Contact, error)

	// List returns contacts for the org, filtered + paginated.
	List(ctx context.Context, orgID organization.ID, filter ContactListFilter) (ContactPage, error)
}
