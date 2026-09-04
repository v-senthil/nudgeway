package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	apptmpl "github.com/v-senthil/nudgeway/internal/application/template"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	tmpldom "github.com/v-senthil/nudgeway/internal/domain/template"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// TemplateDeps bundles the state the template endpoints need.
type TemplateDeps struct {
	// Service is the application-layer use-case. Nil disables all routes.
	Service *apptmpl.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// TemplateDTO is the JSON representation of a persisted template row.
type TemplateDTO struct {
	ID                 string              `json:"id"`
	OrgID              string              `json:"org_id"`
	IntegrationID      string              `json:"integration_id"`
	ProviderTemplateID string              `json:"provider_template_id,omitempty"`
	Name               string              `json:"name"`
	Language           string              `json:"language"`
	Category           string              `json:"category"`
	Status             string              `json:"status"`
	Components         []tmpldom.Component `json:"components"`
	Variables          map[string]string   `json:"variables"`
	LastSyncedAt       *string             `json:"last_synced_at,omitempty"`
	CreatedAt          string              `json:"created_at"`
	UpdatedAt          string              `json:"updated_at"`
}

// TemplateListResponse is the 200 body of GET /api/v1/templates.
type TemplateListResponse struct {
	Items      []TemplateDTO `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// CreateTemplateRequest is the JSON body of POST /api/v1/templates.
type CreateTemplateRequest struct {
	IntegrationID       string              `json:"integration_id"`
	Name                string              `json:"name"`
	Language            string              `json:"language"`
	Category            string              `json:"category"`
	Components          []tmpldom.Component `json:"components"`
	Submit              bool                `json:"submit,omitempty"`
	AllowCategoryChange bool                `json:"allow_category_change,omitempty"`
}

// UpdateTemplateRequest is the JSON body of PUT /api/v1/templates/{id}.
// Only Category + Components are patchable — Name / Language / IntegrationID
// are the natural key on the provider side and stay pinned.
type UpdateTemplateRequest struct {
	Category   string              `json:"category"`
	Components []tmpldom.Component `json:"components"`
}

// SyncTemplatesRequest is the JSON body of POST /api/v1/templates/sync.
type SyncTemplatesRequest struct {
	IntegrationID string `json:"integration_id"`
}

// SyncTemplatesResponse is the 200 body of POST /api/v1/templates/sync.
type SyncTemplatesResponse struct {
	Fetched  int `json:"fetched"`
	Upserted int `json:"upserted"`
}

// mountTemplates installs the /api/v1/templates/* routes on mux.
//
// GETs need auth + templates.read (no CSRF — safe method). Writes need
// auth + CSRF + templates.manage.
func mountTemplates(
	mux Registrar,
	base func(http.Handler) http.Handler,
	authed func(http.Handler) http.Handler,
	deps TemplateDeps,
) {
	if deps.Service == nil {
		return
	}
	h := &templatesHandler{d: deps}
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermTemplatesRead)(next),
		))
	}
	writeGate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermTemplatesManage)(next))
	}
	mux.Handle("GET /api/v1/templates", readGate(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/templates", writeGate(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/v1/templates/{id}", readGate(http.HandlerFunc(h.get)))
	mux.Handle("PUT /api/v1/templates/{id}", writeGate(http.HandlerFunc(h.update)))
	mux.Handle("POST /api/v1/templates/{id}/submit", writeGate(http.HandlerFunc(h.submit)))
	mux.Handle("POST /api/v1/templates/sync", writeGate(http.HandlerFunc(h.sync)))
	mux.Handle("DELETE /api/v1/templates/{id}", writeGate(http.HandlerFunc(h.delete)))
}

// templatesHandler bundles state.
type templatesHandler struct{ d TemplateDeps }

func (h *templatesHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}

// list handles GET /api/v1/templates.
func (h *templatesHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	q := r.URL.Query()
	filter := repository.TemplateListFilter{
		Cursor: q.Get("cursor"),
		Status: tmpldom.Status(q.Get("status")),
	}
	if s := q.Get("integration_id"); s != "" {
		id := integration.ID(s)
		filter.IntegrationID = &id
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeProblem(w, r, http.StatusBadRequest, "validation", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	page, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID), filter)
	if err != nil {
		if errors.Is(err, tmpldom.ErrInvalid) {
			writeProblem(w, r, http.StatusBadRequest, "validation", err.Error())
			return
		}
		h.logger().Warn("template list failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "template list failed")
		return
	}
	resp := TemplateListResponse{
		Items:      make([]TemplateDTO, 0, len(page.Templates)),
		NextCursor: page.NextCursor,
	}
	for _, t := range page.Templates {
		resp.Items = append(resp.Items, toTemplateDTO(t))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// get handles GET /api/v1/templates/{id}.
func (h *templatesHandler) get(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := tmpldom.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	t, err := h.d.Service.Get(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		if errors.Is(err, tmpldom.ErrNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "template not found")
			return
		}
		h.logger().Warn("template get failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("template_id", string(id)),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "template get failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toTemplateDTO(t))
}

// create handles POST /api/v1/templates.
func (h *templatesHandler) create(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	t, err := h.d.Service.Create(r.Context(), organization.ID(pr.OrgID), apptmpl.CreateInput{
		IntegrationID:       req.IntegrationID,
		Name:                req.Name,
		Language:            req.Language,
		Category:            tmpldom.Category(req.Category),
		Components:          req.Components,
		Submit:              req.Submit,
		AllowCategoryChange: req.AllowCategoryChange,
	})
	if err != nil {
		// Service.Create persists the DRAFT before hitting the provider —
		// when submit=true and only the provider call fails, t is the
		// persisted row and err is the provider error. Return 201 with the
		// draft + a submit_error field so the operator sees "saved but
		// submission rejected: <Meta message>" instead of a bare 500.
		if t.ID != "" {
			detail, extras := providerErrorDetail(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			body := map[string]any{
				"template":     toTemplateDTO(t),
				"submit_error": detail,
			}
			for k, v := range extras {
				body[k] = v
			}
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		switch {
		case errors.Is(err, tmpldom.ErrInvalid):
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error())
		case errors.Is(err, tmpldom.ErrIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		default:
			h.logger().Warn("template create failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toTemplateDTO(t))
}

// update handles PUT /api/v1/templates/{id}. Only DRAFT rows are editable.
func (h *templatesHandler) update(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := tmpldom.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	var req UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	t, err := h.d.Service.Update(r.Context(), organization.ID(pr.OrgID), id, apptmpl.UpdateInput{
		Category:   tmpldom.Category(req.Category),
		Components: req.Components,
	})
	if err != nil {
		switch {
		case errors.Is(err, tmpldom.ErrNotFound):
			writeProblem(w, r, http.StatusNotFound, "not_found", "template not found")
		case errors.Is(err, tmpldom.ErrNotEditable):
			writeProblem(w, r, http.StatusConflict, "not_editable",
				"template is not in DRAFT state — clone into a new draft to edit")
		case errors.Is(err, tmpldom.ErrInvalid):
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error())
		default:
			h.logger().Warn("template update failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("template_id", string(id)),
				slog.Any("err", err),
			)
			writeProblem(w, r, http.StatusInternalServerError, "internal", "template update failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toTemplateDTO(t))
}

// submit handles POST /api/v1/templates/{id}/submit.
func (h *templatesHandler) submit(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := tmpldom.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	t, err := h.d.Service.SubmitForReview(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		switch {
		case errors.Is(err, tmpldom.ErrNotFound):
			writeProblem(w, r, http.StatusNotFound, "not_found", "template not found")
		case errors.Is(err, tmpldom.ErrNotSubmittable):
			writeProblem(w, r, http.StatusConflict, "not_submittable",
				"template is not in DRAFT state")
		case errors.Is(err, tmpldom.ErrIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		default:
			h.logger().Warn("template submit failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("template_id", string(id)),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toTemplateDTO(t))
}

// sync handles POST /api/v1/templates/sync.
func (h *templatesHandler) sync(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var req SyncTemplatesRequest
	// Body is optional — a caller with a single integration can POST empty
	// and expect the service to pick the sole integration. We enforce
	// integration_id here because Phase 2 supports multiple WhatsApp
	// integrations per org.
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
	}
	if req.IntegrationID == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "integration_id is required")
		return
	}
	res, err := h.d.Service.Sync(r.Context(), organization.ID(pr.OrgID), integration.ID(req.IntegrationID))
	if err != nil {
		if errors.Is(err, tmpldom.ErrIntegrationMissing) {
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
			return
		}
		h.logger().Warn("template sync failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("integration_id", req.IntegrationID),
			slog.Any("err", err),
		)
		writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SyncTemplatesResponse{
		Fetched:  res.Fetched,
		Upserted: res.Upserted,
	})
}

// delete handles DELETE /api/v1/templates/{id}.
func (h *templatesHandler) delete(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := tmpldom.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	if err := h.d.Service.Delete(r.Context(), organization.ID(pr.OrgID), id); err != nil {
		h.logger().Warn("template delete failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("template_id", string(id)),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "template delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toTemplateDTO flattens a domain Template into the wire shape.
func toTemplateDTO(t tmpldom.Template) TemplateDTO {
	dto := TemplateDTO{
		ID:                 string(t.ID),
		OrgID:              string(t.OrgID),
		IntegrationID:      string(t.IntegrationID),
		ProviderTemplateID: t.ProviderTemplateID,
		Name:               t.Name,
		Language:           t.Language,
		Category:           string(t.Category),
		Status:             string(t.Status),
		Components:         t.Components,
		Variables:          t.Variables,
		CreatedAt:          t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:          t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if dto.Components == nil {
		dto.Components = []tmpldom.Component{}
	}
	if dto.Variables == nil {
		dto.Variables = map[string]string{}
	}
	if t.LastSyncedAt != nil {
		s := t.LastSyncedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		dto.LastSyncedAt = &s
	}
	return dto
}
