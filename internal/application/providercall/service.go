// Package providercall is the application-service layer for the
// operator-facing execution log. It orchestrates truncation, persistence,
// and read-side filtering on behalf of the REST handler and the provider
// tracers.
package providercall

import (
	"context"
	"errors"
	"log/slog"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/providercall"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// DefaultMaxBodyBytes bounds the persisted length of request / response
// bodies. Anything larger is truncated. Kept small enough that a chatty
// send lane doesn't fill MySQL, large enough to include typical provider
// error envelopes with their embedded trace ids / template diagnostics.
const DefaultMaxBodyBytes = 64 << 10 // 65 536 bytes

// Deps bundles the state Service needs.
type Deps struct {
	// Repo persists and queries entries. Required.
	Repo repository.ProviderCallRepo

	// Logger receives warnings when Record silently swallows a persistence
	// failure. Nil is permitted (record errors then go unlogged).
	Logger *slog.Logger

	// MaxBodyBytes overrides DefaultMaxBodyBytes when > 0.
	MaxBodyBytes int
}

// Service is the application-layer entry point for provider-call
// bookkeeping. It has two responsibilities:
//
//  1. Record — called by provider tracers on every outbound HTTP call. Must
//     never propagate errors back to the tracer (the caller's outbound
//     call succeeds regardless of whether the log persisted).
//  2. List — served by the REST handler for operator debugging.
type Service struct {
	repo         repository.ProviderCallRepo
	logger       *slog.Logger
	maxBodyBytes int
}

// NewService constructs a Service from the given Deps. Panics on missing
// Repo — the caller has almost certainly forgotten to wire it.
func NewService(deps Deps) *Service {
	if deps.Repo == nil {
		panic("providercall.NewService: Deps.Repo is required")
	}
	max := deps.MaxBodyBytes
	if max <= 0 {
		max = DefaultMaxBodyBytes
	}
	return &Service{
		repo:         deps.Repo,
		logger:       deps.Logger,
		maxBodyBytes: max,
	}
}

// Record persists an entry. It is fire-and-forget: persistence errors are
// logged (if a logger was configured) and swallowed so a downed MySQL
// never breaks the outbound HTTP path. Request / response bodies are
// truncated to MaxBodyBytes before persist. Direction defaults to
// DirectionOutbound when the caller left it empty.
func (s *Service) Record(ctx context.Context, entry providercall.Entry) {
	if s == nil {
		return
	}
	if entry.Direction == "" {
		entry.Direction = providercall.DirectionOutbound
	}
	entry.RequestBody = truncateBytes(entry.RequestBody, s.maxBodyBytes)
	entry.ResponseBody = truncateBytes(entry.ResponseBody, s.maxBodyBytes)
	// Defensive redaction — no-op today, forward-compatible with future
	// header capture.
	entry = entry.Redact()
	if _, err := s.repo.Record(ctx, entry); err != nil {
		if s.logger != nil {
			s.logger.Warn("providercall: record failed",
				slog.String("provider", entry.Provider),
				slog.String("operation", entry.Operation),
				slog.String("org_id", entry.OrgID),
				slog.Any("err", err),
			)
		}
	}
}

// List returns a page of entries for org filtered by the supplied criteria.
// See repository.ProviderCallListFilter for the field semantics.
func (s *Service) List(ctx context.Context, orgID organization.ID, filter repository.ProviderCallListFilter) ([]providercall.Entry, string, error) {
	if s == nil {
		return nil, "", errors.New("providercall: service not configured")
	}
	return s.repo.List(ctx, orgID, filter)
}

// truncateBytes returns b bounded at max bytes. Returns nil unchanged.
func truncateBytes(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	// Return a copy to avoid aliasing the caller's larger buffer through
	// the persistence layer.
	out := make([]byte, max)
	copy(out, b[:max])
	return out
}
