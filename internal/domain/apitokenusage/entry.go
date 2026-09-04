// Package apitokenusage models the operator-facing execution log for every
// bearer-authenticated request the API serves. Its shape intentionally
// mirrors internal/domain/providercall (outbound provider telemetry) so a
// single UI paradigm covers both directions of external traffic.
//
// Invariants:
//   - Entries are append-only. There is no update path; retention is a
//     future pruning-job responsibility.
//   - Request / response bodies are truncated by the application service
//     before persist; the *_bytes columns preserve the true wire size for
//     accurate metrics.
//   - JSON bodies MUST be redacted for known secret keys (password,
//     access_token, ...) before persist. Redaction is a Service concern —
//     the domain type stores whatever it is handed.
//   - Every entry is org-scoped. Repositories reject queries that do not
//     carry an org predicate.
package apitokenusage

import (
	"errors"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID is the canonical identifier of an api_token_usage row (ULID string form).
type ID string

// NewID returns a fresh ULID-shaped ID suitable for a new usage row.
func NewID() ID { return ID(ulid.Make().String()) }

// Entry is one persisted per-request execution-log row. Zero-values are
// intentional for optional fields — the persistence layer treats empty
// strings / nil byte slices as absent (NULL).
type Entry struct {
	// ID is the row's ULID. Assigned by the application service before
	// persist.
	ID ID

	// OrgID is the tenant boundary. Required.
	OrgID organization.ID

	// TokenID identifies the api_tokens row that authenticated the
	// request. Required.
	TokenID apitoken.ID

	// OccurredAt is the wall-clock timestamp the request completed. Set
	// by the recording middleware; the DB default is a fallback.
	OccurredAt time.Time

	// RequestID stitches the row back to the request-scoped log lines
	// (see middleware/requestid.go).
	RequestID string

	// Method is the HTTP method (GET, POST, ...).
	Method string

	// Path is the request URL path — never includes the query string
	// (may contain PII / tokens on some routes).
	Path string

	// StatusCode is the HTTP status the handler returned.
	StatusCode int

	// LatencyMs is the wall-clock handler duration in milliseconds.
	LatencyMs int

	// RemoteIP is the caller's IP as observed by the server (X-Forwarded-For
	// first hop when present, otherwise RemoteAddr host).
	RemoteIP string

	// UserAgent is the caller's User-Agent header. Optional.
	UserAgent string

	// RequestBody is the captured request body, truncated + JSON-redacted
	// by the application service. May be nil.
	RequestBody []byte

	// ResponseBody is the captured response body, truncated by the
	// application service. May be nil.
	ResponseBody []byte

	// RequestBytes is the true wire size of the request body, before
	// truncation. Used by aggregate metrics.
	RequestBytes int

	// ResponseBytes is the true wire size of the response body, before
	// truncation. Used by aggregate metrics.
	ResponseBytes int

	// ErrorMessage carries a short human-readable failure summary when
	// the handler surfaced a 4xx/5xx problem. Empty on success.
	ErrorMessage string
}

// DailyPoint is one (day, total, errors) triple used to render sparklines
// on the token-detail page.
type DailyPoint struct {
	Day           time.Time
	TotalRequests int64
	ErrorCount    int64
	AvgLatencyMs  int64
	BytesIn       int64
	BytesOut      int64
}

// PathHit summarises the top-N paths a token hit in the query window.
type PathHit struct {
	Path  string
	Count int64
}

// Metrics is the aggregate KPI + series payload returned by
// APITokenUsageRepo.Metrics. All counts are unsigned integers even
// though the field type is int64 so JSON encoding round-trips cleanly.
type Metrics struct {
	// TotalRequests is the count of usage rows in the window.
	TotalRequests int64

	// ErrorCount is the subset of TotalRequests whose status_code is
	// >= 400.
	ErrorCount int64

	// AvgLatencyMs is the mean handler latency across the window.
	AvgLatencyMs int64

	// BytesIn is the sum of RequestBytes across the window.
	BytesIn int64

	// BytesOut is the sum of ResponseBytes across the window.
	BytesOut int64

	// ByDay is the per-day breakdown, oldest-first.
	ByDay []DailyPoint

	// ByStatus is the status-code histogram.
	ByStatus map[int]int64

	// TopPaths lists the most-hit paths, highest-count first (capped
	// at ~10 by the repo).
	TopPaths []PathHit
}

// ErrNotFound is returned by repositories when a lookup for a single row
// misses. The list surface returns an empty page rather than this error.
var ErrNotFound = errors.New("apitokenusage: entry not found")

// ErrInvalidEntry is returned by Record when a required field is empty.
var ErrInvalidEntry = errors.New("apitokenusage: invalid entry")
