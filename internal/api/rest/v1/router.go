package v1

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
)

// Deps bundles everything required to mount the v1 REST API.
type Deps struct {
	Auth               AuthDeps
	PermissionResolver middleware.PermissionResolver
	Logger             *slog.Logger
	SlideEvery         time.Duration
}

// Registrar is the minimal surface Mount needs to install patterns. It is
// satisfied by both *http.ServeMux and infrastructure/http.Server.
type Registrar interface {
	Handle(pattern string, handler http.Handler)
}

// Mount registers every /api/v1/* route on mux, wrapping each with the
// standard fullWA middleware chain.
//
// Middleware order (outermost → innermost):
//
//	RequestID → Recover → Logger → SessionAuth → (RequireAuth) → (RequireCSRF) → handler
//
// RequestID must be first so its ID is on every subsequent log line.
func Mount(mux Registrar, deps Deps) {
	h := &authHandler{d: deps.Auth}

	slide := deps.SlideEvery
	if slide <= 0 {
		slide = 5 * time.Minute
	}

	// Base chain applied to every /api/v1/* endpoint.
	base := func(next http.Handler) http.Handler {
		return middleware.RequestID(
			middleware.Recover(deps.Logger)(
				middleware.Logger(deps.Logger)(
					middleware.SessionAuth(
						deps.Auth.Sessions, deps.Auth.SessionCookie,
						slide, deps.PermissionResolver, deps.Logger,
					)(next),
				),
			),
		)
	}

	// Login: authenticated route (no session yet) that does NOT require CSRF.
	mux.Handle("POST /api/v1/auth/login", base(http.HandlerFunc(h.login)))
	// CSRF bootstrap: unauthenticated read that issues the double-submit cookie.
	mux.Handle("GET /api/v1/auth/csrf", base(http.HandlerFunc(h.csrf)))

	// Authenticated + CSRF-protected routes.
	authed := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequireCSRF(deps.Auth.CSRFCookie)(next),
		))
	}
	mux.Handle("POST /api/v1/auth/logout", authed(http.HandlerFunc(h.logout)))
	// GET /me — CSRF is a no-op on safe methods, but we still gate on auth.
	mux.Handle("GET /api/v1/auth/me", base(middleware.RequireAuth(http.HandlerFunc(h.me))))
}
