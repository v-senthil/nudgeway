package v1

import (
	"log/slog"
	"net/http"
	"time"

	wsapi "github.com/fullwa/fullwa/internal/api/ws"
	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
	fws "github.com/fullwa/fullwa/internal/infrastructure/websocket"
)

// Deps bundles everything required to mount the v1 REST API.
type Deps struct {
	Auth               AuthDeps
	Webhook            WebhookDeps
	Integrations       IntegrationsDeps
	Messages           MessagesDeps
	AttachmentsUpload  AttachmentsUploadDeps
	PermissionResolver middleware.PermissionResolver
	Logger             *slog.Logger
	SlideEvery         time.Duration
	// Hub is the shared per-node WebSocket fan-out. When non-nil, Mount also
	// installs /ws/inbox on the same mux. The endpoint sits directly on the
	// mux (not under /api/v1/*) because the Vite dev proxy handles /ws
	// separately from the REST prefix.
	Hub *fws.Hub
	// WSAllowedOrigins overrides the default set of origins accepted during
	// the WebSocket upgrade. Leave nil to use wsapi.DefaultAllowedOrigins.
	WSAllowedOrigins []string
	// Attachments configures the media serve endpoint
	// (GET/HEAD /api/v1/media/{key}). When Store is nil the route is
	// silently omitted — inbound media persistence is disabled in this
	// deploy.
	Attachments AttachmentsDeps
}

// Registrar is the minimal surface Mount needs to install patterns. It is
// satisfied by both *http.ServeMux and infrastructure/http.Server.
type Registrar interface {
	Handle(pattern string, handler http.Handler)
}

// Mount registers every /api/v1/* route on mux, wrapping each with the
// standard fullWA middleware chain, and installs the provider-agnostic
// /webhooks/* ingress routes with a minimal middleware chain (no auth, no
// CSRF — external providers cannot present our cookies).
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

	// Webhook ingress lives outside /api/v1/* and is intentionally
	// UNAUTHENTICATED at the HTTP layer — external providers cannot
	// present our session cookie or CSRF token. Authenticity is enforced
	// per-provider via signature verification inside the ingress helper.
	webhookBase := func(next http.Handler) http.Handler {
		return middleware.RequestID(
			middleware.Recover(deps.Logger)(
				middleware.Logger(deps.Logger)(next),
			),
		)
	}
	mountWebhooks(mux, webhookBase, deps.Webhook)

	// Integrations REST — auth + integrations.manage; writes also require CSRF.
	if deps.Integrations.Service != nil {
		mountIntegrations(mux, base, authed, deps.Integrations)
	}

	// Messages + conversations REST. POST /messages is auth + CSRF; GETs are
	// auth-only (CSRF is a no-op on safe methods). A conversations index
	// stub is installed only when no peer package has provided the full
	// implementation yet — checked by the wire-up (cmd/server) via the
	// dedicated flag on MessagesDeps.
	authedGET := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(next))
	}
	mountMessages(mux, authed, authedGET, deps.Messages, deps.Messages.IncludeConversationsIndex)

	// Attachments upload — POST /api/v1/attachments (auth + CSRF). The
	// route is silently omitted when no attachments.Store has been wired.
	mountAttachmentsUpload(mux, authed, deps.AttachmentsUpload)

	// Media serve — GET/HEAD /api/v1/media/{key} (auth-only). Streams the
	// bytes for a stored blob keyed by SHA-256. Route is silently omitted
	// when Attachments.Store is nil.
	mountMedia(mux, authedGET, deps.Attachments)

	// /ws/inbox — WebSocket real-time endpoint. Reuses the same session-auth
	// middleware chain as REST so the upgrade sees a Principal on the
	// request context. It is mounted OUTSIDE /api/v1/* so the Vite dev
	// server can proxy /ws separately from the REST prefix.
	if deps.Hub != nil {
		inbox := &wsapi.InboxHandler{
			Hub:            deps.Hub,
			Logger:         deps.Logger,
			AllowedOrigins: deps.WSAllowedOrigins,
		}
		wsChain := base(middleware.RequireAuth(inbox))
		mux.Handle("GET /ws/inbox", wsChain)
	}
}
