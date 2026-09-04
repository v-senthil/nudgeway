package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	appaudit "github.com/v-senthil/nudgeway/internal/application/audit"
	daudit "github.com/v-senthil/nudgeway/internal/domain/audit"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/domain/user"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// AuditDeps bundles the audit endpoints' dependencies.
type AuditDeps struct {
	// Service is the application-layer entry point. Nil disables the
	// route entirely (audit is optional in slim deploys).
	Service *appaudit.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// AuditEntryDTO is the JSON representation of one audit_logs row.
type AuditEntryDTO struct {
	ID           string         `json:"id"`
	OrgID        string         `json:"org_id"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	IP           string         `json:"ip,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	OccurredAt   string         `json:"occurred_at"`
}

// AuditListResponse is the 200 body of GET /api/v1/audit-logs.
type AuditListResponse struct {
	Items      []AuditEntryDTO `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// mountAudit installs GET /api/v1/audit-logs on mux. Route is silently
// omitted when deps.Service is nil so slim deploys can skip the trail.
//
// The authed argument is the caller-supplied middleware chain that gates
// on session + CSRF. GETs skip CSRF (safe method) but keep the auth +
// permission requirements.
func mountAudit(mux Registrar, authed func(http.Handler) http.Handler, deps AuditDeps) {
	if deps.Service == nil {
		return
	}
	h := &auditHandler{d: deps}
	gate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermAuditRead)(next))
	}
	mux.Handle("GET /api/v1/audit-logs", gate(http.HandlerFunc(h.list)))
}

// auditHandler bundles state for the audit endpoints.
type auditHandler struct{ d AuditDeps }

// list handles GET /api/v1/audit-logs.
func (h *auditHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	q := r.URL.Query()

	filter := repository.AuditListFilter{
		ResourceType: q.Get("resource_type"),
		ResourceID:   q.Get("resource_id"),
		Action:       daudit.Action(q.Get("action")),
		Cursor:       q.Get("cursor"),
	}

	if s := q.Get("actor_user_id"); s != "" {
		uid := user.ID(s)
		filter.ActorUserID = &uid
	}
	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "validation", "since must be RFC3339")
			return
		}
		filter.Since = t
	}
	if s := q.Get("until"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "validation", "until must be RFC3339")
			return
		}
		filter.Until = t
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeProblem(w, r, http.StatusBadRequest, "validation", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}

	entries, next, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID), filter)
	if err != nil {
		if errors.Is(err, daudit.ErrInvalidCursor) {
			writeProblem(w, r, http.StatusBadRequest, "validation", "cursor is invalid")
			return
		}
		h.logger().Warn("audit list failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "audit list failed")
		return
	}

	resp := AuditListResponse{
		Items:      make([]AuditEntryDTO, 0, len(entries)),
		NextCursor: next,
	}
	for _, e := range entries {
		resp.Items = append(resp.Items, toAuditDTO(e))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *auditHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}

// toAuditDTO flattens a domain Entry into the wire shape.
func toAuditDTO(e daudit.Entry) AuditEntryDTO {
	dto := AuditEntryDTO{
		ID:           strconv.FormatUint(e.ID, 10),
		OrgID:        string(e.OrgID),
		Action:       string(e.Action),
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		Metadata:     e.Metadata,
		OccurredAt:   e.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	if e.ActorUserID != nil {
		dto.ActorUserID = string(*e.ActorUserID)
	}
	if len(e.IP) > 0 {
		dto.IP = net.IP(e.IP).String()
	}
	return dto
}
