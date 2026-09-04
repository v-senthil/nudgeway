package v1

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	"github.com/v-senthil/nudgeway/internal/ports/attachments"
)

// AttachmentsDeps bundles the runtime dependencies of the media REST
// endpoint. Provided by cmd/server at wire-up time; when Store is nil the
// media route is not mounted (media inbound is disabled in this deploy).
type AttachmentsDeps struct {
	// Store serves the blob bytes for a given key.
	Store attachments.Store
	// Logger receives one structured record per failed lookup.
	Logger *slog.Logger
}

// contentTypeReader is the optional refinement of attachments.Store that
// concrete implementations (like the LocalFS dev store) satisfy to expose
// a persisted MIME type. Kept package-local so the base port stays minimal
// and the handler can degrade to application/octet-stream when the store
// does not know a type for the key.
type contentTypeReader interface {
	ContentType(ctx context.Context, key string) (string, error)
}

// mountMedia registers GET / HEAD /api/v1/media/{key} on mux.
//
// The route is auth-gated (session cookie) so downloaded media stays
// scoped to a signed-in operator; anonymous cross-org access is
// impossible even when the key (a SHA-256 hex string) leaks. CSRF is a
// no-op on safe methods so authedGET is enough.
//
// Response headers:
//
//	Content-Type: <persisted MIME or application/octet-stream fallback>
//	Cache-Control: private, max-age=86400
//
// 404s are served for missing keys or invalid key formats.
func mountMedia(
	mux Registrar,
	authedGET func(http.Handler) http.Handler,
	deps AttachmentsDeps,
) {
	if deps.Store == nil {
		return
	}
	h := &mediaHandler{d: deps}
	mux.Handle("GET /api/v1/media/{key}", authedGET(http.HandlerFunc(h.get)))
	mux.Handle("HEAD /api/v1/media/{key}", authedGET(http.HandlerFunc(h.head)))
}

// mediaHandler bundles state for the media routes.
type mediaHandler struct{ d AttachmentsDeps }

// get streams the bytes for /api/v1/media/{key}.
func (h *mediaHandler) get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "media key required")
		return
	}
	body, err := h.d.Store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeProblem(w, r, http.StatusNotFound, "media_not_found", "media not found")
			return
		}
		h.logger().Warn("media lookup failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("key", key),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "media lookup failed")
		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", h.contentTypeFor(r.Context(), key))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		// Client hung up mid-stream — logged at debug because it's noise.
		h.logger().Debug("media stream aborted",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("key", key),
			slog.Any("err", err),
		)
	}
}

// head serves HEAD /api/v1/media/{key} — headers only, so clients can
// prefetch the content-type without pulling the payload.
func (h *mediaHandler) head(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "media key required")
		return
	}
	body, err := h.d.Store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeProblem(w, r, http.StatusNotFound, "media_not_found", "media not found")
			return
		}
		h.logger().Warn("media lookup failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("key", key),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "media lookup failed")
		return
	}
	_ = body.Close()
	w.Header().Set("Content-Type", h.contentTypeFor(r.Context(), key))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
}

// contentTypeFor returns the MIME type recorded alongside key, falling
// back to application/octet-stream when the store does not know one.
func (h *mediaHandler) contentTypeFor(ctx context.Context, key string) string {
	if s, ok := h.d.Store.(contentTypeReader); ok {
		if ct, err := s.ContentType(ctx, key); err == nil && ct != "" {
			return ct
		}
	}
	return "application/octet-stream"
}

// logger returns the configured slog or the process default.
func (h *mediaHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}
