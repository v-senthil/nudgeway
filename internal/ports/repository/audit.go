package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/audit"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/user"
)

// AuditListFilter narrows an AuditRepo.List query. All fields are optional;
// zero values disable the corresponding predicate. Limit defaults to 50 and
// is capped at 200 to keep the (org, occurred_at) index scan bounded.
type AuditListFilter struct {
	// ActorUserID restricts to entries written on behalf of this user.
	ActorUserID *user.ID
	// ResourceType restricts to a single entity kind (e.g. "integration").
	ResourceType string
	// ResourceID restricts to a single row. Requires ResourceType to hit
	// the composite (org, resource_type, resource_id) index.
	ResourceID string
	// Action restricts to entries with a specific verb.
	Action audit.Action
	// Since restricts to entries with OccurredAt >= Since.
	Since time.Time
	// Until restricts to entries with OccurredAt < Until.
	Until time.Time
	// Limit caps the returned page size. Zero picks the repo default (50).
	Limit int
	// Cursor is the opaque continuation token returned by the previous
	// page. Empty starts from the newest entry.
	Cursor string
}

// AuditRepo persists and reads back append-only AuditLog rows. Writes
// must never fail the caller's request path — see application/audit for
// the wrapper that swallows and logs Record errors.
type AuditRepo interface {
	// Record inserts an Entry and returns its assigned primary key. The
	// implementation stamps OccurredAt with the current UTC time when the
	// caller left it zero. Returns audit.ErrInvalidEntry when OrgID or
	// Action is missing.
	Record(ctx context.Context, e audit.Entry) (uint64, error)

	// List returns a page of entries newest-first for the given org,
	// filtered by AuditListFilter. The returned cursor should be passed
	// back verbatim as the next call's filter.Cursor; empty means the
	// caller reached the end.
	List(ctx context.Context, orgID organization.ID, filter AuditListFilter) ([]audit.Entry, string, error)
}
