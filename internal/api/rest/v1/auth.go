package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	appauth "github.com/v-senthil/nudgeway/internal/application/auth"
	infauth "github.com/v-senthil/nudgeway/internal/infrastructure/auth"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
)

// OrgLookup fetches the display name of an organization by its canonical ID.
type OrgLookup interface {
	Name(ctx context.Context, orgID string) (string, error)
}

// UserLookup fetches the email + display_name for a platform user by ULID.
type UserLookup interface {
	GetProfile(ctx context.Context, userID string) (email, displayName string, err error)
}

// AuthDeps bundles the dependencies of the auth handlers.
type AuthDeps struct {
	Service       *appauth.Service
	Sessions      infauth.SessionStore
	CookieOpts    infauth.CookieOptions
	CSRFOpts      infauth.CookieOptions
	SessionCookie string
	CSRFCookie    string
	Logger        *slog.Logger
	Orgs          OrgLookup
	Users         UserLookup
}

// LoginRequest is the JSON body of POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is the 200 body of POST /api/v1/auth/login.
type LoginResponse struct {
	UserID      string   `json:"user_id"`
	OrgID       string   `json:"org_id"`
	Permissions []string `json:"permissions"`
	ExpiresAt   string   `json:"expires_at"`
}

// Me is the 200 body of GET /api/v1/auth/me.
type Me struct {
	UserID      string   `json:"user_id"`
	OrgID       string   `json:"org_id"`
	OrgName     string   `json:"org_name"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Permissions []string `json:"permissions"`
}

// authHandler bundles the state needed by the individual endpoint funcs.
type authHandler struct{ d AuthDeps }

// login handles POST /api/v1/auth/login.
func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	ip := clientIP(r)
	ua := r.UserAgent()
	res, err := h.d.Service.Login(r.Context(), req.Email, req.Password, ip, ua)
	if err != nil {
		if errors.Is(err, appauth.ErrInvalidCredentials) {
			writeProblem(w, r, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
			return
		}
		h.d.Logger.Warn("login failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "login failed")
		return
	}
	// Session cookie.
	sessOpts := h.d.CookieOpts
	sessOpts.Name = h.d.SessionCookie
	sessOpts.MaxAge = time.Until(res.ExpiresAt)
	infauth.SetSessionCookie(w, res.SessionID, sessOpts)
	// CSRF cookie.
	tok, err := infauth.NewCSRFToken()
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "internal", "csrf token")
		return
	}
	csrfOpts := h.d.CSRFOpts
	csrfOpts.Name = h.d.CSRFCookie
	csrfOpts.MaxAge = time.Until(res.ExpiresAt)
	infauth.SetCSRFCookie(w, tok, csrfOpts)

	perms := make([]string, 0, len(res.Perms))
	for p := range res.Perms {
		perms = append(perms, string(p))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(LoginResponse{
		UserID:      string(res.UserID),
		OrgID:       string(res.OrgID),
		Permissions: perms,
		ExpiresAt:   res.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// logout handles POST /api/v1/auth/logout.
func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(h.d.SessionCookie); err == nil && c.Value != "" {
		if err := h.d.Service.Logout(r.Context(), infauth.SessionID(c.Value)); err != nil {
			h.d.Logger.Warn("logout: session delete failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.Any("err", err),
			)
		}
	}
	sessOpts := h.d.CookieOpts
	sessOpts.Name = h.d.SessionCookie
	infauth.ClearSessionCookie(w, sessOpts)
	// Clear CSRF cookie.
	http.SetCookie(w, &http.Cookie{
		Name: h.d.CSRFCookie, Value: "", Path: "/", MaxAge: -1,
		Secure: h.d.CSRFOpts.Secure, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// me handles GET /api/v1/auth/me.
func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	orgName := ""
	if h.d.Orgs != nil {
		n, err := h.d.Orgs.Name(r.Context(), pr.OrgID)
		if err == nil {
			orgName = n
		}
	}
	email, displayName := "", ""
	if h.d.Users != nil {
		e, dn, err := h.d.Users.GetProfile(r.Context(), pr.UserID)
		if err == nil {
			email, displayName = e, dn
		}
	}
	perms := make([]string, 0, len(pr.Permissions))
	for p := range pr.Permissions {
		perms = append(perms, string(p))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Me{
		UserID: pr.UserID, OrgID: pr.OrgID, OrgName: orgName,
		Email: email, DisplayName: displayName, Permissions: perms,
	})
}

// csrf handles GET /api/v1/auth/csrf and issues a fresh CSRF cookie so that
// the very first login POST can carry an X-CSRF-Token header.
func (h *authHandler) csrf(w http.ResponseWriter, r *http.Request) {
	tok, err := infauth.NewCSRFToken()
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "internal", "csrf token")
		return
	}
	opts := h.d.CSRFOpts
	opts.Name = h.d.CSRFCookie
	if opts.MaxAge <= 0 {
		opts.MaxAge = 24 * time.Hour
	}
	infauth.SetCSRFCookie(w, tok, opts)
	w.WriteHeader(http.StatusNoContent)
}

// clientIP extracts the caller IP: prefers X-Forwarded-For's first entry, else RemoteAddr.
func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		for i := 0; i < len(xf); i++ {
			if xf[i] == ',' {
				return xf[:i]
			}
		}
		return xf
	}
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// writeProblem writes an RFC 7807 problem+json response.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "https://nudgeway.dev/errors/" + title,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"request_id": middleware.RequestIDFrom(r.Context()),
	})
}
