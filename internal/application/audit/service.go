package audit

import (
	"context"
	"log/slog"

	daudit "github.com/v-senthil/nudgeway/internal/domain/audit"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// Deps bundles the application service's dependencies. Repo is required;
// Logger falls back to slog.Default() when nil.
type Deps struct {
	// Repo persists and reads audit entries.
	Repo repository.AuditRepo
	// Logger receives warnings when Record fails. Failures never bubble
	// up to the caller — an audit hiccup must not break a user mutation.
	Logger *slog.Logger
}

// Service is the application-layer audit trail entry point.
type Service struct {
	repo   repository.AuditRepo
	logger *slog.Logger
}

// New constructs a Service. Panics when Repo is nil — wire-up bugs must
// fail loudly at boot instead of silently dropping audit rows.
func New(deps Deps) *Service {
	if deps.Repo == nil {
		panic("application/audit: Repo is required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: deps.Repo, logger: logger}
}

// Record persists a single audit entry. Failures are logged and swallowed:
// the caller (a mutation handler) must not be broken by an audit hiccup.
// This is why the method returns no error — callers can treat it as a
// side-effect they enqueue.
//
// If the caller needs a hard guarantee that the row landed (e.g. legal
// hold), it should call the underlying repository directly.
func (s *Service) Record(ctx context.Context, e daudit.Entry) {
	if s == nil {
		return
	}
	if _, err := s.repo.Record(ctx, e); err != nil {
		s.logger.Warn("audit record failed",
			slog.String("org_id", string(e.OrgID)),
			slog.String("action", string(e.Action)),
			slog.String("resource_type", e.ResourceType),
			slog.String("resource_id", e.ResourceID),
			slog.Any("err", err),
		)
	}
}

// List proxies through to the underlying repository. The REST handler
// converts filter.Limit / filter.Cursor to the repo's shape and formats
// the entries as JSON.
func (s *Service) List(
	ctx context.Context,
	orgID organization.ID,
	filter repository.AuditListFilter,
) ([]daudit.Entry, string, error) {
	return s.repo.List(ctx, orgID, filter)
}
