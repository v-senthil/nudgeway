package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	appgroup "github.com/v-senthil/nudgeway/internal/application/group"
	dgroup "github.com/v-senthil/nudgeway/internal/domain/group"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// GroupsDeps bundles the state the /api/v1/groups handlers need.
type GroupsDeps struct {
	// Service is the application-layer entry point. Nil disables all
	// /api/v1/groups routes.
	Service *appgroup.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// GroupDTO is the JSON representation of a persisted group row.
type GroupDTO struct {
	ID              string         `json:"id"`
	OrgID           string         `json:"org_id"`
	IntegrationID   string         `json:"integration_id"`
	ProviderGroupID string         `json:"provider_group_id"`
	Subject         string         `json:"subject"`
	Description     string         `json:"description,omitempty"`
	Size            int            `json:"size"`
	IsAdmin         bool           `json:"is_admin"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

// GroupListResponse is the 200 body of GET /api/v1/groups.
type GroupListResponse struct {
	Items      []GroupDTO `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// GroupMemberDTO is the JSON representation of a member row.
type GroupMemberDTO struct {
	ID        uint64  `json:"id"`
	GroupID   string  `json:"group_id"`
	ContactID string  `json:"contact_id,omitempty"`
	WaID      string  `json:"wa_id,omitempty"`
	BSUID     string  `json:"bsuid,omitempty"`
	Role      string  `json:"role"`
	JoinedAt  string  `json:"joined_at"`
	LeftAt    *string `json:"left_at,omitempty"`
}

// GroupMemberListResponse is the 200 body of GET /api/v1/groups/{id}/members.
type GroupMemberListResponse struct {
	Items []GroupMemberDTO `json:"items"`
}

// CreateGroupRequest is the JSON body of POST /api/v1/groups.
type CreateGroupRequest struct {
	IntegrationID    string `json:"integration_id"`
	Subject          string `json:"subject"`
	Description      string `json:"description,omitempty"`
	JoinApprovalMode string `json:"join_approval_mode,omitempty"`
}

// SyncGroupsRequest is the JSON body of POST /api/v1/groups/sync.
type SyncGroupsRequest struct {
	IntegrationID string `json:"integration_id"`
}

// SyncGroupsResponse is the 200 body of POST /api/v1/groups/sync.
type SyncGroupsResponse struct {
	GroupsUpserted  int `json:"groups_upserted"`
	MembersUpserted int `json:"members_upserted"`
}

// SendGroupMessageRequest is the JSON body of
// POST /api/v1/groups/{id}/messages. The type + payload envelope mirrors
// SendMessageRequest so operators can reuse the composer shape.
type SendGroupMessageRequest struct {
	Type           string           `json:"type"`
	Text           *json.RawMessage `json:"text,omitempty"`
	Template       *json.RawMessage `json:"template,omitempty"`
	Media          *json.RawMessage `json:"media,omitempty"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"`
}

// MountGroups is the exported wire-up entry point installed by cmd/server
// after Mount returns. It is deliberately additive — the parallel router
// commit does not have to touch router.go for this feature to ship. cmd/
// server calls v1.MountGroups(mux, base, authed, deps) once it has built
// its middleware chain builders.
//
// authed wraps a handler with the standard authenticated + CSRF-protected
// chain; base is the read-only chain builder (auth + permission, no CSRF).
func MountGroups(mux Registrar, base func(http.Handler) http.Handler, authed func(http.Handler) http.Handler, deps GroupsDeps) {
	mountGroups(mux, base, authed, deps)
}

// mountGroups installs the /api/v1/groups routes on mux.
func mountGroups(mux Registrar, base func(http.Handler) http.Handler, authed func(http.Handler) http.Handler, deps GroupsDeps) {
	if deps.Service == nil {
		return
	}
	h := &groupsHandler{d: deps}
	// Reads: auth + permission, no CSRF (safe method).
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermGroupsRead)(next),
		))
	}
	// Manage writes: auth + CSRF + groups.manage.
	manageGate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermGroupsManage)(next))
	}
	// Send writes: auth + CSRF + messages.send.
	sendGate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermMessagesSend)(next))
	}

	mux.Handle("GET /api/v1/groups", readGate(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/groups", manageGate(http.HandlerFunc(h.create)))
	mux.Handle("POST /api/v1/groups/sync", manageGate(http.HandlerFunc(h.sync)))
	mux.Handle("GET /api/v1/groups/{id}", readGate(http.HandlerFunc(h.get)))
	mux.Handle("GET /api/v1/groups/{id}/members", readGate(http.HandlerFunc(h.members)))
	mux.Handle("POST /api/v1/groups/{id}/messages", sendGate(http.HandlerFunc(h.send)))
}

// groupsHandler bundles state for the group endpoints.
type groupsHandler struct{ d GroupsDeps }

// list handles GET /api/v1/groups.
func (h *groupsHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	q := r.URL.Query()
	filter := repository.GroupListFilter{
		IntegrationID: integration.ID(q.Get("integration_id")),
		Query:         q.Get("q"),
		Cursor:        q.Get("cursor"),
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeProblem(w, r, http.StatusBadRequest, "validation", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	rows, next, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID), filter)
	if err != nil {
		h.logger().Warn("groups list failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "groups list failed")
		return
	}
	resp := GroupListResponse{
		Items:      make([]GroupDTO, 0, len(rows)),
		NextCursor: next,
	}
	for _, g := range rows {
		resp.Items = append(resp.Items, toGroupDTO(g))
	}
	writeJSON(w, http.StatusOK, resp)
}

// sync handles POST /api/v1/groups/sync.
func (h *groupsHandler) sync(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var req SyncGroupsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.IntegrationID == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "integration_id is required")
		return
	}
	res, err := h.d.Service.Sync(r.Context(), organization.ID(pr.OrgID), integration.ID(req.IntegrationID))
	if err != nil {
		switch {
		case errors.Is(err, appgroup.ErrNoIntegration):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		case errors.Is(err, appgroup.ErrUnsupported):
			writeProblem(w, r, http.StatusUnprocessableEntity, "unsupported", err.Error())
		default:
			h.logger().Warn("groups sync failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("integration_id", req.IntegrationID),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, SyncGroupsResponse{
		GroupsUpserted:  res.GroupsUpserted,
		MembersUpserted: res.MembersUpserted,
	})
}

// create handles POST /api/v1/groups.
func (h *groupsHandler) create(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.IntegrationID == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "integration_id is required")
		return
	}
	if req.Subject == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "subject is required")
		return
	}
	if len(req.Subject) > 128 {
		writeProblem(w, r, http.StatusBadRequest, "validation", "subject must be ≤128 characters")
		return
	}
	if len(req.Description) > 2048 {
		writeProblem(w, r, http.StatusBadRequest, "validation", "description must be ≤2048 characters")
		return
	}
	switch req.JoinApprovalMode {
	case "", "auto_approve", "approval_required":
	default:
		writeProblem(w, r, http.StatusBadRequest, "validation", "join_approval_mode must be auto_approve or approval_required")
		return
	}
	g, err := h.d.Service.Create(r.Context(), organization.ID(pr.OrgID), appgroup.CreateInput{
		IntegrationID:    integration.ID(req.IntegrationID),
		Subject:          req.Subject,
		Description:      req.Description,
		JoinApprovalMode: req.JoinApprovalMode,
	})
	if err != nil {
		switch {
		case errors.Is(err, appgroup.ErrNoIntegration):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		case errors.Is(err, appgroup.ErrUnsupported):
			writeProblem(w, r, http.StatusUnprocessableEntity, "unsupported", err.Error())
		default:
			h.logger().Warn("groups create failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("integration_id", req.IntegrationID),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, toGroupDTO(g))
}

// get handles GET /api/v1/groups/{id}.
func (h *groupsHandler) get(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := dgroup.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	g, err := h.d.Service.Get(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		if errors.Is(err, appgroup.ErrGroupNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "group not found")
			return
		}
		h.logger().Warn("groups get failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("group_id", string(id)),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "group get failed")
		return
	}
	writeJSON(w, http.StatusOK, toGroupDTO(g))
}

// members handles GET /api/v1/groups/{id}/members.
func (h *groupsHandler) members(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := dgroup.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	rows, err := h.d.Service.Members(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		if errors.Is(err, appgroup.ErrGroupNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "group not found")
			return
		}
		h.logger().Warn("groups members failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("group_id", string(id)),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "members list failed")
		return
	}
	resp := GroupMemberListResponse{Items: make([]GroupMemberDTO, 0, len(rows))}
	for _, m := range rows {
		resp.Items = append(resp.Items, toGroupMemberDTO(m))
	}
	writeJSON(w, http.StatusOK, resp)
}

// send handles POST /api/v1/groups/{id}/messages.
func (h *groupsHandler) send(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := dgroup.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	var req SendGroupMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Type == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "type is required")
		return
	}
	payload, err := selectGroupPayload(req)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation", err.Error())
		return
	}
	reqID := middleware.RequestIDFrom(r.Context())
	res, err := h.d.Service.SendMessage(r.Context(), organization.ID(pr.OrgID), pr.UserID, id, req.Type, payload, req.IdempotencyKey, reqID)
	if err != nil {
		if errors.Is(err, appgroup.ErrGroupNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "group not found")
			return
		}
		h.logger().Warn("groups send failed",
			slog.String("request_id", reqID),
			slog.String("org_id", pr.OrgID),
			slog.String("group_id", string(id)),
			slog.Any("err", err),
		)
		writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message_id": res.MessageID,
		"status":     res.Status,
	})
}

// selectGroupPayload picks the payload sub-object off SendGroupMessageRequest
// based on Type. Same normalisation as the 1:1 message flow so the composer
// can reuse the "bare string means text.body" shorthand.
func selectGroupPayload(req SendGroupMessageRequest) ([]byte, error) {
	switch req.Type {
	case "text":
		if req.Text == nil {
			return nil, errors.New("text payload required for type=text")
		}
		raw := *req.Text
		if len(raw) > 0 && raw[0] == '"' {
			var body string
			if err := json.Unmarshal(raw, &body); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"body": body})
		}
		return raw, nil
	case "template":
		if req.Template == nil {
			return nil, errors.New("template payload required for type=template")
		}
		return *req.Template, nil
	case "image", "video", "audio", "document", "sticker":
		if req.Media == nil {
			return nil, errors.New("media payload required for media type")
		}
		return *req.Media, nil
	default:
		return nil, errors.New("unsupported message type: " + req.Type)
	}
}

// toGroupDTO flattens a domain Group into the wire shape.
func toGroupDTO(g dgroup.Group) GroupDTO {
	dto := GroupDTO{
		ID:              string(g.ID),
		OrgID:           string(g.OrgID),
		IntegrationID:   string(g.IntegrationID),
		ProviderGroupID: g.ProviderGroupID,
		Subject:         g.Subject,
		Description:     g.Description,
		Size:            g.Size,
		IsAdmin:         g.IsAdmin,
		Metadata:        g.Metadata,
		CreatedAt:       g.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       g.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	return dto
}

// toGroupMemberDTO flattens a domain Member.
func toGroupMemberDTO(m dgroup.Member) GroupMemberDTO {
	dto := GroupMemberDTO{
		ID:       m.ID,
		GroupID:  string(m.GroupID),
		WaID:     m.WaID,
		BSUID:    m.BSUID,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.ContactID != nil {
		dto.ContactID = string(*m.ContactID)
	}
	if m.LeftAt != nil {
		s := m.LeftAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		dto.LeftAt = &s
	}
	return dto
}

func (h *groupsHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}
