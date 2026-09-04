package v1

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	appusage "github.com/v-senthil/nudgeway/internal/application/apitokenusage"
	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/apitokenusage"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// APITokenUsageDeps bundles the state the api-token usage handlers need.
type APITokenUsageDeps struct {
	// Service is the application-layer entry point. Nil silently omits
	// the routes so slim deploys can skip usage tracking entirely.
	Service *appusage.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// apiTokenUsageHandler bundles state for the individual endpoint funcs.
type apiTokenUsageHandler struct{ d APITokenUsageDeps }

// mountAPITokenUsage installs the /api/v1/api-tokens/{id}/usage +
// /metrics routes on mux. Both are auth-gated on session or bearer;
// callers see only usage rows belonging to their own org (repos scope
// on org_id).
func mountAPITokenUsage(mux Registrar, base func(http.Handler) http.Handler, deps APITokenUsageDeps) {
	if deps.Service == nil {
		return
	}
	h := &apiTokenUsageHandler{d: deps}
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(next))
	}
	mux.Handle("GET /api/v1/api-tokens/{id}/usage", readGate(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/api-tokens/{id}/metrics", readGate(http.HandlerFunc(h.metrics)))
}

// apiTokenUsageEntryDTO is the JSON shape of one usage row. Bodies are
// exposed both base64 (canonical, size-preserving) and best-effort UTF-8
// text so the UI can pretty-print JSON payloads without a second round-
// trip.
type apiTokenUsageEntryDTO struct {
	ID               string `json:"id"`
	TokenID          string `json:"token_id"`
	OccurredAt       string `json:"occurred_at"`
	RequestID        string `json:"request_id"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	StatusCode       int    `json:"status_code"`
	LatencyMs        int    `json:"latency_ms"`
	RemoteIP         string `json:"remote_ip"`
	UserAgent        string `json:"user_agent,omitempty"`
	RequestBody      string `json:"request_body,omitempty"`  // base64
	ResponseBody     string `json:"response_body,omitempty"` // base64
	RequestBodyText  string `json:"request_body_text,omitempty"`
	ResponseBodyText string `json:"response_body_text,omitempty"`
	RequestBytes     int    `json:"request_bytes"`
	ResponseBytes    int    `json:"response_bytes"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// apiTokenUsageListResponse wraps a page for a self-describing JSON shape.
type apiTokenUsageListResponse struct {
	Items      []apiTokenUsageEntryDTO `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

// apiTokenDailyPointDTO is one (day, counts) tuple in the metrics series.
type apiTokenDailyPointDTO struct {
	Day           string `json:"day"`
	TotalRequests int64  `json:"total_requests"`
	ErrorCount    int64  `json:"error_count"`
	AvgLatencyMs  int64  `json:"avg_latency_ms"`
	BytesIn       int64  `json:"bytes_in"`
	BytesOut      int64  `json:"bytes_out"`
}

// apiTokenPathHitDTO is one entry in the top-paths ranking.
type apiTokenPathHitDTO struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// apiTokenMetricsDTO is the JSON body of GET /api/v1/api-tokens/{id}/metrics.
type apiTokenMetricsDTO struct {
	TotalRequests int64                   `json:"total_requests"`
	ErrorCount    int64                   `json:"error_count"`
	AvgLatencyMs  int64                   `json:"avg_latency_ms"`
	BytesIn       int64                   `json:"bytes_in"`
	BytesOut      int64                   `json:"bytes_out"`
	ByDay         []apiTokenDailyPointDTO `json:"by_day"`
	ByStatus      map[string]int64        `json:"by_status"`
	TopPaths      []apiTokenPathHitDTO    `json:"top_paths"`
}

// list handles GET /api/v1/api-tokens/{id}/usage.
func (h *apiTokenUsageHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	tokenID := apitoken.ID(r.PathValue("id"))
	if tokenID == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}

	q := r.URL.Query()
	filter := repository.UsageFilter{
		TokenID: &tokenID,
		Cursor:  q.Get("cursor"),
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
		h.logErr(r, "list", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "api-token usage list failed")
		return
	}
	items := make([]apiTokenUsageEntryDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, toUsageDTO(e))
	}
	writeJSON(w, http.StatusOK, apiTokenUsageListResponse{Items: items, NextCursor: next})
}

// metrics handles GET /api/v1/api-tokens/{id}/metrics.
func (h *apiTokenUsageHandler) metrics(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	tokenID := apitoken.ID(r.PathValue("id"))
	if tokenID == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	q := r.URL.Query()
	var from, to time.Time
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid since (want RFC3339)")
			return
		}
		from = t
	}
	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid until (want RFC3339)")
			return
		}
		to = t
	}
	m, err := h.d.Service.Metrics(r.Context(), organization.ID(pr.OrgID), tokenID, from, to)
	if err != nil {
		h.logErr(r, "metrics", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "api-token metrics failed")
		return
	}
	writeJSON(w, http.StatusOK, toMetricsDTO(m))
}

// toUsageDTO maps a domain entry to the wire JSON shape.
func toUsageDTO(e apitokenusage.Entry) apiTokenUsageEntryDTO {
	dto := apiTokenUsageEntryDTO{
		ID:            string(e.ID),
		TokenID:       string(e.TokenID),
		OccurredAt:    e.OccurredAt.UTC().Format(time.RFC3339Nano),
		RequestID:     e.RequestID,
		Method:        e.Method,
		Path:          e.Path,
		StatusCode:    e.StatusCode,
		LatencyMs:     e.LatencyMs,
		RemoteIP:      e.RemoteIP,
		UserAgent:     e.UserAgent,
		RequestBytes:  e.RequestBytes,
		ResponseBytes: e.ResponseBytes,
		ErrorMessage:  e.ErrorMessage,
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
			dto.ResponseBodyText = string(e.ResponseBody)
		}
	}
	return dto
}

// toMetricsDTO maps the domain metrics to the wire JSON shape.
func toMetricsDTO(m apitokenusage.Metrics) apiTokenMetricsDTO {
	dto := apiTokenMetricsDTO{
		TotalRequests: m.TotalRequests,
		ErrorCount:    m.ErrorCount,
		AvgLatencyMs:  m.AvgLatencyMs,
		BytesIn:       m.BytesIn,
		BytesOut:      m.BytesOut,
		ByStatus:      map[string]int64{},
		ByDay:         make([]apiTokenDailyPointDTO, 0, len(m.ByDay)),
		TopPaths:      make([]apiTokenPathHitDTO, 0, len(m.TopPaths)),
	}
	for _, d := range m.ByDay {
		dto.ByDay = append(dto.ByDay, apiTokenDailyPointDTO{
			Day:           d.Day.UTC().Format("2006-01-02"),
			TotalRequests: d.TotalRequests,
			ErrorCount:    d.ErrorCount,
			AvgLatencyMs:  d.AvgLatencyMs,
			BytesIn:       d.BytesIn,
			BytesOut:      d.BytesOut,
		})
	}
	for code, c := range m.ByStatus {
		dto.ByStatus[strconv.Itoa(code)] = c
	}
	for _, p := range m.TopPaths {
		dto.TopPaths = append(dto.TopPaths, apiTokenPathHitDTO{Path: p.Path, Count: p.Count})
	}
	return dto
}

// logErr logs a handler-level error with the standard field set.
func (h *apiTokenUsageHandler) logErr(r *http.Request, op string, err error) {
	if h.d.Logger == nil {
		return
	}
	h.d.Logger.Warn("api_token_usage handler error",
		slog.String("op", op),
		slog.String("request_id", middleware.RequestIDFrom(r.Context())),
		slog.Any("err", err),
	)
}
