// Command fullwa is the single-binary entrypoint for the fullWA platform.
//
// It boots the HTTP server, wires MySQL + Redis, mounts the v1 REST API,
// registers health probes, and shuts everything down gracefully on
// SIGINT/SIGTERM. All external configuration is loaded from
// config/local.yaml (or the path in FULLWA_CONFIG), with FULLWA_* env
// vars overriding individual keys.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appauth "github.com/fullwa/fullwa/internal/application/auth"
	v1 "github.com/fullwa/fullwa/internal/api/rest/v1"
	infauth "github.com/fullwa/fullwa/internal/infrastructure/auth"
	"github.com/fullwa/fullwa/internal/infrastructure/config"
	"github.com/fullwa/fullwa/internal/infrastructure/health"
	fhttp "github.com/fullwa/fullwa/internal/infrastructure/http"
	fmysql "github.com/fullwa/fullwa/internal/infrastructure/mysql"
	fredis "github.com/fullwa/fullwa/internal/infrastructure/redis"
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

	// --- MySQL --------------------------------------------------------------
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := fmysql.Open(dbCtx, cfg.MySQL)
	dbCancel()
	if err != nil {
		return fmt.Errorf("mysql: %w", err)
	}
	logger.Info("mysql connected")

	// --- Redis --------------------------------------------------------------
	rdb := fredis.New(cfg.Redis)
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	if err := fredis.Ping(pingCtx, rdb); err != nil {
		pingCancel()
		logger.Warn("redis ping failed at boot", slog.Any("err", err))
	} else {
		logger.Info("redis connected")
	}
	pingCancel()

	// --- Repositories + services -------------------------------------------
	users := fmysql.NewUsers(db)
	sessions := fmysql.NewSessions(db)
	perms := fmysql.NewRBAC(db)
	orgs := fmysql.NewOrgs(db)

	argParams := infauth.Argon2Params{
		MemoryKiB:   cfgArgonMem(),
		Iterations:  3,
		Parallelism: 2,
		SaltBytes:   16,
		KeyBytes:    32,
	}
	_ = argParams // params consumed by the CLI on user creation; kept for future rehash.

	authSvc := appauth.NewService(users, sessions, perms.AsTyped(), cfg.Auth.SessionTTL)

	secure := cfg.Env == "prod" || cfg.Env == "staging"
	cookieOpts := infauth.CookieOptions{
		Path:     "/",
		MaxAge:   cfg.Auth.SessionTTL,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	csrfOpts := infauth.CookieOptions{
		Path:     "/",
		MaxAge:   cfg.Auth.SessionTTL,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}

	// --- HTTP server --------------------------------------------------------
	srv := fhttp.NewServer(cfg.HTTP, logger)

	v1.Mount(srv, v1.Deps{
		Auth: v1.AuthDeps{
			Service:       authSvc,
			Sessions:      sessions,
			CookieOpts:    cookieOpts,
			CSRFOpts:      csrfOpts,
			SessionCookie: cfg.Auth.SessionCookieName,
			CSRFCookie:    cfg.Auth.CSRFCookieName,
			Logger:        logger,
			Orgs:          orgs,
		},
		PermissionResolver: perms,
		Logger:             logger,
		SlideEvery:         5 * time.Minute,
	})

	// Liveness.
	srv.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	// Readiness.
	probes := []health.Probe{
		health.MySQLProbe(db),
		health.RedisProbe(rdb),
	}
	srv.Handle("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, results := health.Ready(r.Context(), 2*time.Second, probes)
		status := http.StatusOK
		if !ok {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  statusStr(ok),
			"probes":  results,
		})
	}))
	// Metrics placeholder.
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
		logger.Error("http shutdown", slog.Any("err", err))
	}
	if err := rdb.Close(); err != nil {
		logger.Error("redis close", slog.Any("err", err))
	}
	if err := db.Close(); err != nil {
		logger.Error("mysql close", slog.Any("err", err))
	}
	logger.Info("bye")
	return nil
}

// statusStr renders a boolean readiness as a stable string.
func statusStr(ok bool) string {
	if ok {
		return "ready"
	}
	return "not_ready"
}

// cfgArgonMem returns the default argon2 memory. Kept as a function so we can
// later plug in per-config overrides without changing call sites.
func cfgArgonMem() uint32 { return 64 * 1024 }

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
