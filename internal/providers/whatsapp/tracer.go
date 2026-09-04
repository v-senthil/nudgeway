package whatsapp

import "context"

// Tracer receives one TraceEvent per outbound HTTP call the adapter makes
// to Meta. The default implementation (NopTracer) discards events; the
// real wiring in cmd/server closes over a providercall.Service.Record.
//
// Tracers must return quickly and must not block the caller — they run
// on the same goroutine as the outbound HTTP call. Persistence is fire-
// and-forget by contract; a downed database MUST NOT break the outbound
// path.
type Tracer interface {
	// OnCall is invoked exactly once per completed HTTP round-trip
	// (successful or failed). It is also invoked when the request never
	// left the wire (transport error) — in that case StatusCode is 0
	// and ErrClass / ErrMessage carry the diagnostic.
	OnCall(ctx context.Context, event TraceEvent)
}

// TraceEvent is the payload the client hands to the tracer on every call.
// All fields are simple values so the tracer implementation can persist
// them without cross-package type coupling.
//
// Note: TraceEvent intentionally does NOT carry a headers map. The adapter
// stamps only the Authorization / Content-Type headers on outbound calls;
// those must NEVER be logged, and no other headers are attached today.
type TraceEvent struct {
	// Operation names the adapter method (see providercall.Op* constants).
	Operation string

	// Method is the HTTP verb sent (GET, POST, ...).
	Method string

	// URL is the fully-qualified request URL.
	URL string

	// RequestBody is the raw request body. Nil for GETs. The tracer is
	// responsible for truncation before persist.
	RequestBody []byte

	// ResponseBody is the raw response body. Intentionally nil for the
	// download_media operation — the raw bytes are the media itself.
	ResponseBody []byte

	// StatusCode is the HTTP status. Zero when the request never
	// completed (transport error).
	StatusCode int

	// LatencyMs is the wall-clock duration in milliseconds.
	LatencyMs int64

	// ErrClass classifies the failure ("transient", "rate_limited",
	// "auth", "permanent", "unknown"). Empty on success.
	ErrClass string

	// ErrMessage is a short human-readable failure message.
	ErrMessage string

	// TraceID is Meta's fbtrace_id (from the response header or the
	// error envelope). Empty when unavailable.
	TraceID string

	// IntegrationID is the ULID of the integration whose Config drove
	// this call. Empty when the config is not tied to a persisted row.
	IntegrationID string

	// OrgID is the tenant boundary. Empty when the caller (typically
	// a unit test) did not thread one through.
	OrgID string
}

// NopTracer is the zero-value tracer — every call is discarded. It is the
// default when Config.Tracer is nil so unit tests never crash on a nil
// dereference.
type NopTracer struct{}

// OnCall implements Tracer by doing nothing.
func (NopTracer) OnCall(context.Context, TraceEvent) {}
