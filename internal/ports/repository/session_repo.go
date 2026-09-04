package repository

import (
	"context"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
)

// SessionRepo persists Session rows.
type SessionRepo interface {
	// FindOrCreateActive returns the single ACTIVE session for the
	// (org, endpoint, contact) tuple, creating one if none exists.
	// Implementations rely on the partial unique index enforced by the
	// migration to prevent duplicates under concurrency.
	FindOrCreateActive(
		ctx context.Context,
		orgID organization.ID,
		endpointID session.BusinessEndpointID,
		contactID contact.ID,
	) (session.Session, error)

	// Get fetches a session by (OrgID, ID).
	Get(ctx context.Context, orgID organization.ID, id session.ID) (session.Session, error)

	// Close transitions a session to closed. Idempotent: closing an
	// already-closed session returns nil.
	Close(ctx context.Context, orgID organization.ID, id session.ID) error
}
