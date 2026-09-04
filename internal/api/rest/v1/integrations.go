package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	appintegration "github.com/v-senthil/nudgeway/internal/application/integration"
	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
)

// IntegrationsDeps bundles the state the integration handlers need.
type IntegrationsDeps struct {
	// Service is the application-layer use-case entry point.
	Service *appintegration.Service
	// PublicBaseURL is the externally-reachable base (e.g.
	// "https://app.example.com") used to build webhook URLs the
	// operator pastes into provider consoles. Trailing slash is stripped.
	PublicBaseURL string
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// integrationsHandler bundles the state the individual endpoint funcs need.
type integrationsHandler struct{ d IntegrationsDeps }

// mountIntegrations installs the /api/v1/integrations/* routes on mux.
// All routes require an authenticated session and the
// `integrations.manage` permission; state-changing routes also require
// a valid CSRF double-submit cookie.
func mountIntegrations(mux Registrar, base func(http.Handler) http.Handler, authed func(http.Handler) http.Handler, deps IntegrationsDeps) {
	h := &integrationsHandler{d: deps}
	// GETs: auth + permission, no CSRF (safe method).
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermIntegrationsManage)(next),
		))
	}
	// Writes: auth + CSRF + permission.
	writeGate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermIntegrationsManage)(next))
	}

	mux.Handle("GET /api/v1/integrations", readGate(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/integrations", writeGate(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/v1/integrations/{id}", readGate(http.HandlerFunc(h.get)))
	mux.Handle("POST /api/v1/integrations/{id}/test", writeGate(http.HandlerFunc(h.test)))
	mux.Handle("DELETE /api/v1/integrations/{id}", writeGate(http.HandlerFunc(h.delete)))
}

// createIntegrationRequest is the JSON body of POST /api/v1/integrations.
type createIntegrationRequest struct {
	Type     string            `json:"type"`
	Provider string            `json:"provider"`
	Name     string            `json:"name"`
	Config   map[string]any    `json:"config"`
	Secrets  map[string]string `json:"secrets"`
}

// integrationListResponse wraps a slice for a self-describing JSON shape.
type integrationListResponse struct {
	Items []appintegration.PublicIntegration `json:"items"`
}

// list handles GET /api/v1/integrations.
func (h *integrationsHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	items, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID))
	if err != nil {
		h.logErr(r, "list", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "integration list failed")
		return
	}
	for i := range items {
		items[i].WebhookURL = h.webhookURL(items[i].Provider, dintegration.ID(items[i].ID))
	}
	writeJSON(w, http.StatusOK, integrationListResponse{Items: items})
}

// get handles GET /api/v1/integrations/{id}.
func (h *integrationsHandler) get(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id := dintegration.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	item, err := h.d.Service.Get(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		if errors.Is(err, appintegration.ErrNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "integration not found")
			return
		}
		h.logErr(r, "get", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "integration get failed")
		return
	}
	item.WebhookURL = h.webhookURL(item.Provider, dintegration.ID(item.ID))
	writeJSON(w, http.StatusOK, item)
}

// create handles POST /api/v1/integrations.
func (h *integrationsHandler) create(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	var req createIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	in := appintegration.CreateInput{
		Type:     dintegration.Type(req.Type),
		Provider: req.Provider,
		Name:     req.Name,
		Config:   req.Config,
		Secrets:  req.Secrets,
	}
	item, _, err := h.d.Service.Create(r.Context(), organization.ID(pr.OrgID), in)
	if err != nil {
		var vErr *appintegration.ErrValidation
		if errors.As(err, &vErr) {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", vErr.Error())
			return
		}
		h.logErr(r, "create", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "integration create failed")
		return
	}
	item.WebhookURL = h.webhookURL(item.Provider, dintegration.ID(item.ID))
	writeJSON(w, http.StatusCreated, item)
}

// test handles POST /api/v1/integrations/{id}/test.
func (h *integrationsHandler) test(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id := dintegration.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	res, err := h.d.Service.Test(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		if errors.Is(err, appintegration.ErrNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "integration not found")
			return
		}
		h.logErr(r, "test", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "integration test failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// delete handles DELETE /api/v1/integrations/{id}.
func (h *integrationsHandler) delete(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id := dintegration.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	if err := h.d.Service.Delete(r.Context(), organization.ID(pr.OrgID), id); err != nil {
		if errors.Is(err, appintegration.ErrNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "integration not found")
			return
		}
		h.logErr(r, "delete", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "integration delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// webhookURL builds the fully-qualified provider webhook URL. When
// PublicBaseURL is empty (dev mode) the caller receives a relative path,
// which is still meaningful to a curl paste but obviously non-portable.
func (h *integrationsHandler) webhookURL(providerKey string, id dintegration.ID) string {
	path := appintegration.WebhookPath(providerKey, id)
	base := strings.TrimRight(h.d.PublicBaseURL, "/")
	if base == "" {
		return path
	}
	return base + path
}

// logErr logs a handler-level error with the standard field set.
func (h *integrationsHandler) logErr(r *http.Request, op string, err error) {
	if h.d.Logger == nil {
		return
	}
	h.d.Logger.Warn("integrations handler error",
		slog.String("op", op),
		slog.String("request_id", middleware.RequestIDFrom(r.Context())),
		slog.Any("err", err),
	)
}

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
