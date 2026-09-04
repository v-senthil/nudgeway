package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// APITokenRepo persists api_tokens rows.
//
// Implementations enforce tenant isolation on every read + write — callers
// must pass the caller's OrgID; a token belonging to a different org is
// treated as not found.
type APITokenRepo interface {
	// Create inserts a new token row. Callers populate Prefix, SecretHash,
	// Name, ExpiresAt, OrgID, UserID, ID, CreatedAt.
	Create(ctx context.Context, t apitoken.Token) error

	// ListByOrg returns every token owned by the org, newest-first.
	// Includes revoked tokens (the UI badges them separately).
	ListByOrg(ctx context.Context, orgID organization.ID) ([]apitoken.Token, error)

	// LookupByPrefix returns the token whose plaintext prefix matches.
	// Returns apitoken.ErrNotFound when no row matches.
	LookupByPrefix(ctx context.Context, prefix string) (apitoken.Token, error)

	// TouchLastUsed stamps last_used_at for a token. Best-effort: callers
	// invoke it asynchronously and ignore the error.
	TouchLastUsed(ctx context.Context, id apitoken.ID, when time.Time) error

	// Revoke stamps revoked_at for a token owned by orgID. Returns
	// apitoken.ErrNotFound when no matching row exists. Idempotent —
	// revoking an already-revoked token is a no-op.
	Revoke(ctx context.Context, orgID organization.ID, id apitoken.ID, when time.Time) error
}
