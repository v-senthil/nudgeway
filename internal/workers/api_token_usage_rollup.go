package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// APITokenUsageRollupDeps bundles the runner's dependencies.
type APITokenUsageRollupDeps struct {
	// Repo is the read+write surface for the usage rollup table.
	// Required.
	Repo repository.APITokenUsageRepo
	// Orgs enumerates the tenants to visit. Required.
	Orgs OrgLister
	// Logger is required — the runner refuses silent operation.
	Logger *slog.Logger
	// Interval controls the tick cadence. Defaults to 15 minutes when
	// left zero.
	Interval time.Duration
}

// APITokenUsageRollupRunner rolls up yesterday, today, and tomorrow of
// api_token_usage rows into api_token_usage_daily for every org on a
// cron-style tick. Owns one goroutine (the ticker loop); no unbounded
// fan-out.
//
// The 3-day sliding window matches AnalyticsRollupRunner: yesterday so
// late-arriving requests near midnight converge into the correct day,
// today for the live window, tomorrow as a safety net across small tz
// drift.
type APITokenUsageRollupRunner struct {
	repo     repository.APITokenUsageRepo
	orgs     OrgLister
	log      *slog.Logger
	interval time.Duration
}

// NewAPITokenUsageRollupRunner constructs a Runner. Panics on missing
// required deps — wire-up bugs must fail loudly at boot.
func NewAPITokenUsageRollupRunner(deps APITokenUsageRollupDeps) *APITokenUsageRollupRunner {
	if deps.Repo == nil {
		panic("workers: api-token usage rollup: Repo is required")
	}
	if deps.Orgs == nil {
		panic("workers: api-token usage rollup: Orgs is required")
	}
	if deps.Logger == nil {
		panic("workers: api-token usage rollup: Logger is required")
	}
	interval := deps.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &APITokenUsageRollupRunner{
		repo:     deps.Repo,
		orgs:     deps.Orgs,
		log:      deps.Logger,
		interval: interval,
	}
}

// Run blocks until ctx is cancelled, ticking every Interval. Each tick
// rolls up yesterday, today, and tomorrow for every enumerated org.
// Errors are logged + swallowed so a broken tenant doesn't block the
// rest of the org list; RollupDay is idempotent so the next tick still
// catches up.
func (r *APITokenUsageRollupRunner) Run(ctx context.Context) error {
	// Immediate first tick so bootstrap doesn't have to wait interval
	// minutes before showing any data.
	r.tick(ctx)

	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			r.tick(ctx)
		}
	}
}

// tick performs one rollup pass across every enumerated org.
func (r *APITokenUsageRollupRunner) tick(ctx context.Context) {
	now := time.Now()
	today := dayStart(now)
	yesterday := today.Add(-24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	orgs, err := r.orgs.ListOrgIDs(ctx)
	if err != nil {
		r.log.WarnContext(ctx, "api-token usage rollup: list orgs",
			slog.Any("err", err),
		)
		return
	}

	for _, orgID := range orgs {
		for _, d := range []time.Time{yesterday, today, tomorrow} {
			if err := r.repo.RollupDay(ctx, orgID, d); err != nil {
				r.log.WarnContext(ctx, "api-token usage rollup",
					slog.String("org_id", string(orgID)),
					slog.Time("day", d),
					slog.Any("err", err),
				)
			}
		}
	}
}
