package workers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	appanalytics "github.com/v-senthil/nudgeway/internal/application/analytics"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// AnalyticsRollupJobName is the bookmark key persisted in
// analytics_rollup_state. Kept exported so an admin CLI can inspect it
// without importing this file's private constants.
const AnalyticsRollupJobName = "analytics.rollup.daily"

// OrgLister enumerates the tenants a job should iterate. The rollup
// runner takes this as an injected port so the same worker code can
// drive tests (in-memory list), local dev (single-org seed) and prod
// (paginated org repo) without conditional logic in the worker.
type OrgLister interface {
	// ListOrgIDs returns the tenants the rollup should visit. Callers
	// treat the returned slice as a snapshot — the worker re-invokes
	// this on every tick so newly-created tenants are picked up.
	ListOrgIDs(ctx context.Context) ([]organization.ID, error)
}

// AnalyticsRollupDeps bundles the runner's dependencies.
type AnalyticsRollupDeps struct {
	// Service is the application-layer entry point. Required.
	Service *appanalytics.Service
	// Orgs enumerates the tenants to visit. Required.
	Orgs OrgLister
	// Repo is used to persist + restore the job bookmark. Required.
	Repo repository.AnalyticsRepo
	// Logger is required — the runner refuses silent operation.
	Logger *slog.Logger
	// Interval controls the tick cadence. Defaults to 15 minutes when
	// left zero.
	Interval time.Duration
}

// AnalyticsRollupRunner rolls up yesterday + today for every org on a
// cron-style tick. It owns one goroutine (the ticker loop); no
// unbounded fan-out.
type AnalyticsRollupRunner struct {
	svc      *appanalytics.Service
	orgs     OrgLister
	repo     repository.AnalyticsRepo
	log      *slog.Logger
	interval time.Duration
}

// NewAnalyticsRollupRunner constructs a Runner. Panics on missing
// required deps — wire-up bugs must fail loudly at boot.
func NewAnalyticsRollupRunner(deps AnalyticsRollupDeps) *AnalyticsRollupRunner {
	if deps.Service == nil {
		panic("workers: analytics rollup: Service is required")
	}
	if deps.Orgs == nil {
		panic("workers: analytics rollup: Orgs is required")
	}
	if deps.Repo == nil {
		panic("workers: analytics rollup: Repo is required")
	}
	if deps.Logger == nil {
		panic("workers: analytics rollup: Logger is required")
	}
	interval := deps.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &AnalyticsRollupRunner{
		svc:      deps.Service,
		orgs:     deps.Orgs,
		repo:     deps.Repo,
		log:      deps.Logger,
		interval: interval,
	}
}

// Run blocks until ctx is cancelled, ticking every Interval. Each tick
// rolls up yesterday and today for every enumerated org — both days
// are computed to catch late-arriving webhook updates that mutate a
// message's status after midnight UTC.
//
// The runner never spawns unbounded goroutines: it processes orgs
// sequentially on the ticker's goroutine. If a single tick takes
// longer than the interval, the ticker drops the extra tick — but the
// next tick still catches up because Rollup is idempotent.
func (r *AnalyticsRollupRunner) Run(ctx context.Context) error {
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

// tick performs one rollup pass. All errors are logged and swallowed —
// a broken tenant must not block the rest of the org list. The next
// tick will retry.
//
// Uses local time (not UTC) so the window boundaries align with
// MySQL's CURRENT_TIMESTAMP DEFAULT, which is session-tz-relative.
// Rolling today + yesterday + tomorrow gives us a safety net across
// small tz drift (DST, server tz reconfig).
func (r *AnalyticsRollupRunner) tick(ctx context.Context) {
	now := time.Now()
	today := dayStart(now)
	yesterday := today.Add(-24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	orgs, err := r.orgs.ListOrgIDs(ctx)
	if err != nil {
		r.log.WarnContext(ctx, "analytics rollup: list orgs",
			slog.Any("err", err),
		)
		return
	}

	for _, orgID := range orgs {
		for _, d := range []time.Time{yesterday, today, tomorrow} {
			if err := r.svc.Rollup(ctx, orgID, d); err != nil {
				r.log.WarnContext(ctx, "analytics rollup",
					slog.String("org_id", string(orgID)),
					slog.Time("day", d),
					slog.Any("err", err),
				)
			}
		}
	}
	if err := r.repo.SaveRollupState(ctx, AnalyticsRollupJobName, today); err != nil {
		r.log.WarnContext(ctx, "analytics rollup: save state",
			slog.Any("err", err),
		)
	}
}

// LastProcessedDay returns the runner's persisted bookmark. Exposed
// for smoke tests + an admin health probe.
func (r *AnalyticsRollupRunner) LastProcessedDay(ctx context.Context) (time.Time, error) {
	day, err := r.repo.RollupState(ctx, AnalyticsRollupJobName)
	if err != nil {
		return time.Time{}, fmt.Errorf("analytics rollup state: %w", err)
	}
	return day, nil
}

// dayStart returns t truncated to midnight in t's own Location. Local
// time matches how MySQL's CURRENT_TIMESTAMP default inserts data when
// the server tz isn't UTC.
func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
