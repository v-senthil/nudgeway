package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/providercall"
)

// ProviderCallListFilter narrows the ProviderCallRepo.List query surface.
// All fields are optional; zero values mean "no filter on that axis".
type ProviderCallListFilter struct {
	// IntegrationID scopes the query to a single integration when set.
	IntegrationID *integration.ID

	// Operation filters on the exact adapter operation name (e.g.
	// "send_message"). Empty string means no filter.
	Operation string

	// StatusCodeMin / StatusCodeMax bound the HTTP status range. Both
	// zero means no filter. StatusCodeMax=0 with StatusCodeMin>0 means
	// "everything at or above StatusCodeMin".
	StatusCodeMin int
	StatusCodeMax int

	// Since / Until bound the occurred_at range. Zero time values mean
	// unbounded on that side.
	Since time.Time
	Until time.Time

	// Cursor is an opaque continuation token returned by the previous
	// call. Empty means "start at the newest row".
	Cursor string

	// Limit caps the number of rows returned. Implementations clamp to
	// [1, 200] with a default of 50 when zero.
	Limit int
}

// ProviderCallRepo persists and queries provider-call execution log rows.
//
// The recording path (Record) is fire-and-forget from the caller's
// perspective — implementations must NEVER cause a caller's outbound HTTP
// call to fail. The application service wraps Record to ensure that
// contract; implementations should still return meaningful errors so the
// wrapper can log them.
type ProviderCallRepo interface {
	// Record inserts a new entry and returns the assigned auto-increment
	// ID. The entry's OccurredAt is honored when non-zero; otherwise the
	// database default (CURRENT_TIMESTAMP(3)) applies.
	Record(ctx context.Context, entry providercall.Entry) (uint64, error)

	// List returns a page of entries for org, newest-first, plus an
	// opaque next-cursor. An empty cursor in the response means the
	// caller has reached the end.
	List(ctx context.Context, orgID organization.ID, filter ProviderCallListFilter) ([]providercall.Entry, string, error)
}
