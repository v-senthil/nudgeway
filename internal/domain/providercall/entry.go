// Package providercall models the operator-facing execution log for every
// outbound HTTP call the provider adapters make. It is deliberately
// provider-agnostic — the fact that today's only writer is one specific
// adapter is a wiring detail, not a domain invariant. Any adapter that
// wires its own tracer lands entries in the same table with a different
// Provider value.
//
// Invariants:
//   - Entries are append-only. There is no update path; retention is enforced
//     by a Phase 4 pruning job.
//   - Authorization headers, Bearer tokens, or any secret material MUST NOT
//     be stored. Persistence takes only request / response BODIES, and the
//     application-service layer truncates them at MaxBodyBytes.
//   - Every entry is org-scoped. Queries without an org_id predicate are
//     forbidden by the port interface.
package providercall

import "time"

// Direction distinguishes calls the platform made outbound from calls that
// came inbound. Only DirectionOutbound is used today; DirectionInbound is
// reserved for a future refactor that migrates webhook_events onto this
// same table.
type Direction string

// Direction values.
const (
	// DirectionOutbound is a call we made to the provider (POST /messages,
	// GET /media, etc.).
	DirectionOutbound Direction = "outbound"

	// DirectionInbound is reserved for future consolidation with
	// webhook_events. Not yet written by any code path.
	DirectionInbound Direction = "inbound"
)

// Operation enumerates the canonical operation names emitted by the adapters.
// The constants are string aliases (not enums) so a new adapter can add its
// own operations without a cross-cutting schema change.
type Operation string

// Well-known operation values used by the initial adapter set. Other
// adapters may define their own constants — the persisted column is a
// free-form VARCHAR(64).
const (
	OpSendMessage       Operation = "send_message"
	OpMarkAsRead        Operation = "mark_as_read"
	OpGetMediaURL       Operation = "get_media_url"
	OpDownloadMedia     Operation = "download_media"
	OpListTemplates     Operation = "list_templates"
	OpCreateTemplate    Operation = "create_template"
	OpGetTemplateStatus Operation = "get_template_status"
	OpUploadMedia       Operation = "upload_media"
)

// Entry is one persisted execution-log row. Zero-values are intentional for
// optional fields — the persistence layer treats empty strings as absent.
type Entry struct {
	// ID is the internal auto-increment identifier. Zero on entries that
	// have not yet been persisted.
	ID uint64

	// OrgID is the tenant boundary. Required.
	OrgID string

	// IntegrationID identifies which integration produced the call. May be
	// empty when a very-early failure prevents the integration row from
	// being loaded.
	IntegrationID string

	// Provider is the registry key of the adapter that emitted the
	// entry. Required.
	Provider string

	// Operation names the adapter method (see the Op* constants above).
	Operation string

	// Direction distinguishes outbound / inbound. Defaults to outbound.
	Direction Direction

	// Method is the HTTP method (GET, POST, ...).
	Method string

	// URL is the fully-qualified URL the adapter called. May include the
	// version prefix — never contains secrets.
	URL string

	// StatusCode is the HTTP status returned by the provider. Zero when
	// the request never completed (transport error).
	StatusCode int

	// LatencyMs is the wall-clock duration of the request in milliseconds.
	LatencyMs int64

	// RequestBody is the raw request body sent (truncated per MaxBodyBytes).
	// May be nil for GETs.
	RequestBody []byte

	// ResponseBody is the raw response body received (truncated per
	// MaxBodyBytes). Intentionally nil for download_media entries — the
	// raw bytes are the media itself, not useful for debugging.
	ResponseBody []byte

	// ErrorClass tags the failure class ("transient", "rate_limited",
	// "auth", "permanent", "unknown"). Empty on success.
	ErrorClass string

	// ErrorMessage is the short human-readable failure message.
	ErrorMessage string

	// TraceID is the provider-side trace identifier.
	TraceID string

	// CorrelationID stitches the entry back to the originating request
	// / job / event. Empty when the caller did not thread one through.
	CorrelationID string

	// OccurredAt is the wall-clock timestamp of the call. Set by the
	// application layer, not the caller — the DB default is a fallback.
	OccurredAt time.Time
}

// Redact returns a copy of e with any secret material scrubbed. Today the
// entity does not carry secret headers, so Redact is a no-op — the method is
// kept for future-proofing so callers can wrap it defensively.
//
// If a future refactor lets adapters attach headers, add the scrubbing here
// (Authorization, Cookie, X-API-Key, ...). The invariant is: whatever comes
// out of Redact is safe to log or to serve over the read API.
func (e Entry) Redact() Entry {
	// No secret-bearing fields today. Return the entry unchanged.
	return e
}
