package v1

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	appproviderc "github.com/v-senthil/nudgeway/internal/application/providercall"
	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/providercall"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ProviderCallsDeps bundles the state the provider-calls handler needs.
type ProviderCallsDeps struct {
	// Service is the application-layer entry point. Nil silently
	// omits the route.
	Service *appproviderc.Service

	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// providerCallsHandler bundles the state each endpoint func needs.
type providerCallsHandler struct{ d ProviderCallsDeps }

// mountProviderCalls installs the /api/v1/provider-calls route on mux.
// Auth + integrations.manage permission (there is no dedicated
// "integrations.read" today — read + manage are gated on the same key).
func mountProviderCalls(mux Registrar, base func(http.Handler) http.Handler, deps ProviderCallsDeps) {
	if deps.Service == nil {
		return
	}
	h := &providerCallsHandler{d: deps}
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermIntegrationsManage)(next),
		))
	}
	mux.Handle("GET /api/v1/provider-calls", readGate(http.HandlerFunc(h.list)))
}

// providerCallDTO is the JSON shape the operator UI consumes. It mirrors
// providercall.Entry but hex-encodes the raw bodies so JSON parsers don't
// choke on binary content, and returns ULIDs as strings.
type providerCallDTO struct {
	ID              uint64 `json:"id"`
	OrgID           string `json:"org_id"`
	IntegrationID   string `json:"integration_id,omitempty"`
	Provider        string `json:"provider"`
	Operation       string `json:"operation"`
	Direction       string `json:"direction"`
	Method          string `json:"method"`
	URL             string `json:"url"`
	StatusCode      int    `json:"status_code"`
	LatencyMs       int64  `json:"latency_ms"`
	RequestBody     string `json:"request_body,omitempty"`  // base64
	ResponseBody    string `json:"response_body,omitempty"` // base64
	RequestBodyText string `json:"request_body_text,omitempty"`
	ResponseText    string `json:"response_body_text,omitempty"`
	ErrorClass      string `json:"error_class,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
	TraceID         string `json:"trace_id,omitempty"`
	CorrelationID   string `json:"correlation_id,omitempty"`
	OccurredAt      string `json:"occurred_at"`
}

// providerCallListResponse wraps a page for a self-describing JSON shape.
type providerCallListResponse struct {
	Items      []providerCallDTO `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// list handles GET /api/v1/provider-calls.
func (h *providerCallsHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	q := r.URL.Query()

	filter := repository.ProviderCallListFilter{
		Operation: q.Get("operation"),
		Cursor:    q.Get("cursor"),
	}
	if v := q.Get("integration_id"); v != "" {
		id := dintegration.ID(v)
		filter.IntegrationID = &id
	}
	if v := q.Get("status_min"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid status_min")
			return
		}
		filter.StatusCodeMin = n
	}
	if v := q.Get("status_max"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid status_max")
			return
		}
		filter.StatusCodeMax = n
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid since (want RFC3339)")
			return
		}
		filter.Since = t
	}
	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid until (want RFC3339)")
			return
		}
		filter.Until = t
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 200 {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid limit (1..200)")
			return
		}
		filter.Limit = n
	}

	entries, next, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID), filter)
	if err != nil {
		if h.d.Logger != nil {
			h.d.Logger.Warn("provider_calls list failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.Any("err", err),
			)
		}
		writeProblem(w, r, http.StatusInternalServerError, "internal", "provider_calls list failed")
		return
	}

	items := make([]providerCallDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, toProviderCallDTO(e))
	}
	writeJSON(w, http.StatusOK, providerCallListResponse{Items: items, NextCursor: next})
}

// toProviderCallDTO maps a domain entry to the JSON shape. Raw bodies are
// exposed both as base64 (canonical, size-preserving) and as best-effort
// UTF-8 text — most Meta responses are JSON, so the UI can pretty-print
// without a second round-trip.
func toProviderCallDTO(e providercall.Entry) providerCallDTO {
	dto := providerCallDTO{
		ID:            e.ID,
		OrgID:         e.OrgID,
		IntegrationID: e.IntegrationID,
		Provider:      e.Provider,
		Operation:     e.Operation,
		Direction:     string(e.Direction),
		Method:        e.Method,
		URL:           e.URL,
		StatusCode:    e.StatusCode,
		LatencyMs:     e.LatencyMs,
		ErrorClass:    e.ErrorClass,
		ErrorMessage:  e.ErrorMessage,
		TraceID:       e.TraceID,
		CorrelationID: e.CorrelationID,
		OccurredAt:    e.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	if len(e.RequestBody) > 0 {
		dto.RequestBody = base64.StdEncoding.EncodeToString(e.RequestBody)
		if isProbablyText(e.RequestBody) {
			dto.RequestBodyText = string(e.RequestBody)
		}
	}
	if len(e.ResponseBody) > 0 {
		dto.ResponseBody = base64.StdEncoding.EncodeToString(e.ResponseBody)
		if isProbablyText(e.ResponseBody) {
			dto.ResponseText = string(e.ResponseBody)
		}
	}
	return dto
}

// isProbablyText heuristically decides whether a byte slice is safe to
// return as a UTF-8 string in JSON. Refuses NULs / control chars outside
// the whitespace set. Meta responses are always JSON so this passes.
func isProbablyText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c == 0 {
			return false
		}
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
	}
	return true
}

