// Command fullwa is the single-binary entrypoint for the fullWA platform.
//
// It boots the HTTP + WebSocket server, background workers, scheduler, event
// bus, webhook ingress, and provider adapters — wired together per the
// dependency rule in .go-arch-lint.yml. All external configuration is loaded
// from config/local.yaml (or the path in FULLWA_CONFIG), with FULLWA_* env
// vars overriding individual keys.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fullwa/fullwa/internal/infrastructure/config"
	fhttp "github.com/fullwa/fullwa/internal/infrastructure/http"
)

// version is stamped at build time via `-ldflags "-X main.version=..."`.
var version = "0.0.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", envOr("FULLWA_CONFIG", "config/local.yaml"), "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Log)
	slog.SetDefault(logger)
	logger.Info("fullwa starting",
		slog.String("version", version),
		slog.String("env", cfg.Env),
		slog.String("config", *cfgPath),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := fhttp.NewServer(cfg.HTTP, logger)
	// Health probes.
	srv.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	srv.Handle("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Phase 0: no downstream dependencies wired yet. Later phases will
		// probe MySQL, Redis, HBase, and mark not-ready on failure.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	// Metrics endpoint placeholder — Prometheus handler will land in Phase 0
	// once the metrics registry is wired.
	srv.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# metrics not yet wired\n"))
	}))

	logger.Info("http listen", slog.String("addr", cfg.HTTP.Addr))
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("bye")
	return nil
}

func newLogger(l config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	switch l.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level, AddSource: l.AddSource}
	if l.Format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// startedAt is retained as a build-time hook for future readiness diagnostics.
var _ = time.Now
