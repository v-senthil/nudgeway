package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appapitoken "github.com/v-senthil/nudgeway/internal/application/apitoken"
	dapitoken "github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/user"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
)

// APITokensDeps bundles the state the API-token handlers need.
type APITokensDeps struct {
	// Service is the application-layer use-case entry point.
	Service *appapitoken.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// apiTokensHandler bundles the state the individual endpoint funcs need.
type apiTokensHandler struct{ d APITokensDeps }

// mountAPITokens installs the /api/v1/api-tokens routes on mux. All routes
// require an authenticated caller (session cookie or bearer token). List
// is safe-method (CSRF is a no-op); create + revoke go through the CSRF
// gate — bearer-authenticated callers are exempt by the middleware.
func mountAPITokens(mux Registrar, base func(http.Handler) http.Handler, authed func(http.Handler) http.Handler, deps APITokensDeps) {
	if deps.Service == nil {
		return
	}
	h := &apiTokensHandler{d: deps}
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(next))
	}
	mux.Handle("GET /api/v1/api-tokens", readGate(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/api-tokens", authed(http.HandlerFunc(h.create)))
	mux.Handle("DELETE /api/v1/api-tokens/{id}", authed(http.HandlerFunc(h.revoke)))
}

// createAPITokenRequest is the JSON body of POST /api/v1/api-tokens.
type createAPITokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays *int   `json:"expires_in_days,omitempty"`
}

// apiTokenView is the safe list-view projection of a token.
type apiTokenView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// createAPITokenResponse is the shape returned by POST /api/v1/api-tokens.
// `plaintext` is included ONLY on this call — never persisted, never
// returned again.
type createAPITokenResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Plaintext string     `json:"plaintext"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// apiTokenListResponse wraps a slice for a self-describing JSON shape.
type apiTokenListResponse struct {
	Items []apiTokenView `json:"items"`
}

// list handles GET /api/v1/api-tokens.
func (h *apiTokensHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	items, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID))
	if err != nil {
		h.logErr(r, "list", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "api-token list failed")
		return
	}
	out := make([]apiTokenView, 0, len(items))
	for _, t := range items {
		out = append(out, viewOf(t))
	}
	writeJSON(w, http.StatusOK, apiTokenListResponse{Items: out})
}

// create handles POST /api/v1/api-tokens.
func (h *apiTokensHandler) create(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	var req createAPITokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "name required")
		return
	}
	if l := len([]rune(name)); l > 120 {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "name too long (max 120 chars)")
		return
	}
	var expiresIn *time.Duration
	if req.ExpiresInDays != nil {
		if *req.ExpiresInDays <= 0 {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "expires_in_days must be positive")
			return
		}
		d := time.Duration(*req.ExpiresInDays) * 24 * time.Hour
		expiresIn = &d
	}
	item, plaintext, err := h.d.Service.Create(r.Context(),
		organization.ID(pr.OrgID),
		user.ID(pr.UserID),
		name, expiresIn,
	)
	if err != nil {
		var vErr *appapitoken.ErrValidation
		if errors.As(err, &vErr) {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", vErr.Error())
			return
		}
		h.logErr(r, "create", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "api-token create failed")
		return
	}
	writeJSON(w, http.StatusCreated, createAPITokenResponse{
		ID:        string(item.ID),
		Name:      item.Name,
		Prefix:    item.Prefix,
		Plaintext: plaintext,
		CreatedAt: item.CreatedAt,
		ExpiresAt: item.ExpiresAt,
	})
}

// revoke handles DELETE /api/v1/api-tokens/{id}.
func (h *apiTokensHandler) revoke(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id := dapitoken.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	if err := h.d.Service.Revoke(r.Context(), organization.ID(pr.OrgID), id); err != nil {
		if errors.Is(err, dapitoken.ErrNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "api token not found")
			return
		}
		h.logErr(r, "revoke", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "api-token revoke failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// viewOf projects a PublicToken to the wire-JSON shape.
func viewOf(t appapitoken.PublicToken) apiTokenView {
	return apiTokenView{
		ID:         string(t.ID),
		Name:       t.Name,
		Prefix:     t.Prefix,
		CreatedAt:  t.CreatedAt,
		LastUsedAt: t.LastUsedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
	}
}

// logErr logs a handler-level error with the standard field set.
func (h *apiTokensHandler) logErr(r *http.Request, op string, err error) {
	if h.d.Logger == nil {
		return
	}
	h.d.Logger.Warn("api_tokens handler error",
		slog.String("op", op),
		slog.String("request_id", middleware.RequestIDFrom(r.Context())),
		slog.Any("err", err),
	)
}
