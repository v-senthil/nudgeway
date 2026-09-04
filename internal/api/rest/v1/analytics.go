package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	appanalytics "github.com/v-senthil/nudgeway/internal/application/analytics"
	danalytics "github.com/v-senthil/nudgeway/internal/domain/analytics"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
)

// AnalyticsDeps bundles the state the analytics endpoints need.
type AnalyticsDeps struct {
	// Service is the application-layer entry point. Nil silently
	// omits the routes so slim deploys can skip analytics entirely.
	Service *appanalytics.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// OverviewDTO is the JSON body of GET /api/v1/analytics/overview.
type OverviewDTO struct {
	MessagesTotal           int64 `json:"messages_total"`
	DeliveryRatePct         int64 `json:"delivery_rate_pct"`
	ResponseTimeSecondsP50  int64 `json:"response_time_seconds_p50"`
	ConversationsOpened     int64 `json:"conversations_opened"`
	CallsTotal              int64 `json:"calls_total"`
	CallsAnswered           int64 `json:"calls_answered"`
	CallsAvgDurationSeconds int64 `json:"calls_avg_duration_seconds"`
}

// PointDTO is one (day, value) pair inside a SeriesDTO.
type PointDTO struct {
	// Day is the ISO calendar date (YYYY-MM-DD) in UTC.
	Day string `json:"day"`
	// Value is the integer aggregate for the day.
	Value int64 `json:"value"`
}

// SeriesDTO is the JSON body of GET /api/v1/analytics/series.
type SeriesDTO struct {
	Name   string     `json:"name"`
	Points []PointDTO `json:"points"`
}

// mountAnalytics installs GET /api/v1/analytics/overview and
// GET /api/v1/analytics/series on mux. Both are auth-gated + require
// the `analytics.read` permission. Routes are silently omitted when
// deps.Service is nil.
//
// authedGET is the caller-supplied middleware chain that gates on
// session (safe methods skip CSRF).
func mountAnalytics(mux Registrar, authedGET func(http.Handler) http.Handler, deps AnalyticsDeps) {
	if deps.Service == nil {
		return
	}
	h := &analyticsHandler{d: deps}
	gate := func(next http.Handler) http.Handler {
		return authedGET(middleware.RequirePermission(rbac.PermAnalyticsRead)(next))
	}
	mux.Handle("GET /api/v1/analytics/overview", gate(http.HandlerFunc(h.overview)))
	mux.Handle("GET /api/v1/analytics/series", gate(http.HandlerFunc(h.series)))
}

// analyticsHandler bundles state for the analytics endpoints.
type analyticsHandler struct{ d AnalyticsDeps }

// overview handles GET /api/v1/analytics/overview?from=&to=.
func (h *analyticsHandler) overview(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}
	ov, err := h.d.Service.Overview(r.Context(), organization.ID(pr.OrgID), from, to)
	if err != nil {
		if errors.Is(err, danalytics.ErrInvalidRange) {
			writeProblem(w, r, http.StatusBadRequest, "validation", "invalid range")
			return
		}
		h.logger().Warn("analytics overview failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "analytics overview failed")
		return
	}
	writeJSON(w, http.StatusOK, OverviewDTO{
		MessagesTotal:           ov.MessagesTotal,
		DeliveryRatePct:         ov.DeliveryRatePct,
		ResponseTimeSecondsP50:  ov.ResponseTimeSecondsP50,
		ConversationsOpened:     ov.ConversationsOpened,
		CallsTotal:              ov.CallsTotal,
		CallsAnswered:           ov.CallsAnswered,
		CallsAvgDurationSeconds: ov.CallsAvgDurationSeconds,
	})
}

// series handles GET /api/v1/analytics/series?kind=&from=&to=&provider=.
func (h *analyticsHandler) series(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	q := r.URL.Query()
	kind := danalytics.SeriesKind(q.Get("kind"))
	if kind == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "kind is required")
		return
	}
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}
	series, err := h.d.Service.Series(r.Context(), organization.ID(pr.OrgID), kind, from, to, q.Get("provider"))
	if err != nil {
		switch {
		case errors.Is(err, danalytics.ErrUnknownSeries):
			writeProblem(w, r, http.StatusBadRequest, "validation", "unknown series kind")
			return
		case errors.Is(err, danalytics.ErrInvalidRange):
			writeProblem(w, r, http.StatusBadRequest, "validation", "invalid range")
			return
		}
		h.logger().Warn("analytics series failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("kind", string(kind)),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "analytics series failed")
		return
	}
	pts := make([]PointDTO, 0, len(series.Points))
	for _, p := range series.Points {
		pts = append(pts, PointDTO{Day: p.Day.UTC().Format("2006-01-02"), Value: p.Value})
	}
	writeJSON(w, http.StatusOK, SeriesDTO{Name: series.Name, Points: pts})
}

// parseDateRange parses the shared ?from=&to= YYYY-MM-DD query
// parameters. Writes a 400 problem and returns ok=false on any invalid
// value.
func parseDateRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	q := r.URL.Query()
	fromStr := q.Get("from")
	toStr := q.Get("to")
	if fromStr == "" || toStr == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "from and to are required (YYYY-MM-DD)")
		return time.Time{}, time.Time{}, false
	}
	// Parse in Local so the range boundaries align with MySQL's
	// session-tz-relative CURRENT_TIMESTAMP defaults (which is how
	// created_at is stamped).
	from, err := time.ParseInLocation("2006-01-02", fromStr, time.Local)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation", "from must be YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	to, err := time.ParseInLocation("2006-01-02", toStr, time.Local)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation", "to must be YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	if from.After(to) {
		writeProblem(w, r, http.StatusBadRequest, "validation", "from must be <= to")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func (h *analyticsHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}
