package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/apitokenusage"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// UsageFilter narrows the APITokenUsageRepo.List query surface. All fields
// are optional; zero values mean "no filter on that axis".
type UsageFilter struct {
	// TokenID scopes the query to a single api-token. Optional — omit
	// to list every bearer-auth call across every token in the org.
	TokenID *apitoken.ID

	// StatusCodeMin / StatusCodeMax bound the HTTP status range. Both
	// zero means no filter.
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

// APITokenUsageRepo persists + queries per-request usage rows plus the
// daily rollup projection.
//
// Record is fire-and-forget from the caller's perspective — the recording
// middleware runs it in a detached goroutine, so slow persistence must
// never block the caller's HTTP response. Implementations should still
// return meaningful errors so the wrapper can log them.
type APITokenUsageRepo interface {
	// Record inserts one usage row. The entry's OccurredAt is honored
	// when non-zero; otherwise the database default applies.
	Record(ctx context.Context, e apitokenusage.Entry) error

	// List returns a page of entries newest-first, plus an opaque
	// next-cursor. An empty cursor in the response means the caller has
	// reached the end.
	List(ctx context.Context, orgID organization.ID, filter UsageFilter) ([]apitokenusage.Entry, string, error)

	// Metrics returns the aggregate KPIs, per-day series, per-status
	// histogram, and top-paths breakdown for one token over
	// [from, to]. Implementations prefer the api_token_usage_daily
	// rollup for series data and fall back to the raw log for the
	// live-tail portion.
	Metrics(ctx context.Context, orgID organization.ID, tokenID apitoken.ID, from, to time.Time) (apitokenusage.Metrics, error)

	// RollupDay recomputes and upserts every api_token_usage_daily row
	// for orgID on the given (UTC) day. Idempotent: repeat calls with
	// the same arguments produce the same table state.
	RollupDay(ctx context.Context, orgID organization.ID, day time.Time) error
}
