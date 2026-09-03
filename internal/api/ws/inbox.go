package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"

	fws "github.com/fullwa/fullwa/internal/infrastructure/websocket"
	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
	"nhooyr.io/websocket"
)

// InboxHandler serves GET /ws/inbox. It upgrades an authenticated HTTP
// request into a per-org WebSocket connection registered on the shared Hub.
type InboxHandler struct {
	// Hub is the process-wide fan-out surface. Required.
	Hub *fws.Hub
	// Logger is used for structured connection lifecycle logs. If nil,
	// slog.Default() is used.
	Logger *slog.Logger
	// AllowedOrigins is the list of Origin patterns accepted by the upgrade.
	// If empty, DefaultAllowedOrigins is used.
	AllowedOrigins []string
	// ClientOptions tunes the per-connection ping / write / buffer knobs. Zero
	// values in the struct fall back to package defaults.
	ClientOptions fws.ClientOptions
}

// DefaultAllowedOrigins matches the Vite dev server and the embedded prod
// origin. Additional origins should be configured explicitly at wire time
// rather than being added here.
var DefaultAllowedOrigins = []string{
	"localhost:5173",
	"localhost:8080",
	"[::1]:5173",
	"127.0.0.1:5173",
	"127.0.0.1:8080",
}

// helloFrame is the first message every client receives after upgrade so the
// UI can confirm the session it thinks it has matches what the server sees.
type helloFrame struct {
	Type    string `json:"type"`
	OrgID   string `json:"org_id"`
	UserID  string `json:"user_id"`
	Version int    `json:"version"`
}

// ServeHTTP implements http.Handler. The middleware chain must have already
// populated a Principal on the request context; unauthenticated requests are
// rejected with 401 before any upgrade attempt.
func (h *InboxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}

	origins := h.AllowedOrigins
	if len(origins) == 0 {
		origins = DefaultAllowedOrigins
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: origins,
		// InsecureSkipVerify defaults to false — do NOT flip it.
	})
	if err != nil {
		logger.Warn("ws accept failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("user_id", pr.UserID),
			slog.Any("err", err),
		)
		return
	}

	room := h.Hub.Room(pr.OrgID)
	client := fws.NewClient(conn, room, pr.OrgID, pr.UserID, logger, h.ClientOptions)

	// First frame: the server's view of who is connected. Fired via Enqueue
	// so it flows through the same write pump as broadcast frames.
	if payload, err := json.Marshal(helloFrame{
		Type:    "hello",
		OrgID:   pr.OrgID,
		UserID:  pr.UserID,
		Version: 1,
	}); err == nil {
		client.Enqueue(payload)
	}

	logger.Info("ws inbox connected",
		slog.String("request_id", middleware.RequestIDFrom(r.Context())),
		slog.String("org_id", pr.OrgID),
		slog.String("user_id", pr.UserID),
	)

	// Blocks until the connection is closed by either side or ctx cancellation.
	client.Run(r.Context())

	logger.Info("ws inbox disconnected",
		slog.String("org_id", pr.OrgID),
		slog.String("user_id", pr.UserID),
		slog.Uint64("dropped", client.Dropped()),
	)
}

// writeProblem writes an RFC 7807 problem+json response. Duplicated from
// the REST layer to avoid pulling a REST-package dependency into the WS
// endpoint.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "https://fullwa.dev/errors/" + title,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"request_id": middleware.RequestIDFrom(r.Context()),
	})
}
