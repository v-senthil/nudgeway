// Package http wraps the stdlib HTTP server with Nudgeway defaults, middleware
// composition, and graceful shutdown.
//
// Phase 0 exposes only the raw mux and health probes. Phase 1 adds the
// middleware chain (request-ID, auth, RBAC, tenant, rate-limit, recover,
// logger) and mounts the generated REST + WebSocket handlers.
package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/v-senthil/nudgeway/internal/infrastructure/config"
)

// Server is a thin wrapper around *http.Server with a mux and structured
// logging. It uses net/http.ServeMux (Go 1.22+ method-aware routing) so we
// carry no HTTP-framework dependency in Phase 0.
type Server struct {
	http *http.Server
	mux  *http.ServeMux
	log  *slog.Logger
}

// NewServer builds a Server from HTTPConfig. Callers register handlers via
// Handle(pattern, handler) and then invoke ListenAndServe.
func NewServer(cfg config.HTTPConfig, log *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux: mux,
		log: log,
		http: &http.Server{
			Addr:              cfg.Addr,
			Handler:           mux,
			ReadHeaderTimeout: cfg.ReadTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}
	return s
}

// Handle registers a handler for a Go 1.22 method-aware pattern (e.g. "GET /healthz").
func (s *Server) Handle(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

// ListenAndServe blocks until the underlying server returns an error.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully terminates the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
