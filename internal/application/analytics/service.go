package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	danalytics "github.com/v-senthil/nudgeway/internal/domain/analytics"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// providerAll is the sentinel used for pan-provider aggregate rows.
// Kept lower-case + short so it matches the DEFAULT declared in the
// migration (analytics_messages_daily.provider DEFAULT 'all').
const providerAll = "all"

// typeAll is the sentinel used for pan-type aggregate rows in
// analytics_messages_daily.message_type.
const typeAll = "all"

// Deps bundles the service's dependencies. All fields are required.
type Deps struct {
	// Repo persists and reads the rollup tables.
	Repo repository.AnalyticsRepo
	// Raw reads the canonical `messages` + `conversations` tables to
	// feed the rollup pipeline.
	Raw repository.AnalyticsSource
	// Logger receives warnings. Falls back to slog.Default when nil.
	Logger *slog.Logger
}

// Service is the application-layer entry point for the analytics
// pipeline. It exposes:
//
//   - Rollup — recompute one day's aggregates from raw data (idempotent).
//   - Overview — dashboard summary cards over a date range.
//   - Series — a labelled time series for a line chart.
type Service struct {
	repo   repository.AnalyticsRepo
	raw    repository.AnalyticsSource
	logger *slog.Logger
}

// New constructs a Service. Panics on missing required deps — wire-up
// bugs must fail loudly at boot.
func New(deps Deps) *Service {
	if deps.Repo == nil {
		panic("application/analytics: Repo is required")
	}
	if deps.Raw == nil {
		panic("application/analytics: Raw is required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: deps.Repo, raw: deps.Raw, logger: logger}
}

// Rollup recomputes every analytics rollup row for the given org and
// UTC day. It is idempotent: repeat calls with the same (orgID, day)
// produce the same table state because every write is an upsert on the
// composite PRIMARY KEY.
//
// The pipeline is:
//
//  1. Read raw per-(provider, direction, type) counts from `messages`.
//  2. Fold detail rows into a MessagesDaily slice, plus synthetic
//     (provider='all', message_type='all') aggregate rows per direction.
//  3. Persist to analytics_messages_daily.
//  4. Aggregate outbound rows into DeliveryRateDaily per provider
//     (plus 'all'), persist to analytics_delivery_rate_daily.
//  5. Read conversations opened/resolved + p50 response time, persist
//     to analytics_conversations_daily.
//
// day is truncated to the UTC date; any time-of-day component is
// discarded.
func (s *Service) Rollup(ctx context.Context, orgID organization.ID, day time.Time) error {
	if day.IsZero() {
		return danalytics.ErrInvalidRollupDay
	}
	utcDay := day.UTC()
	dayOnly := time.Date(utcDay.Year(), utcDay.Month(), utcDay.Day(), 0, 0, 0, 0, time.UTC)

	breakdowns, err := s.raw.CountMessagesByDay(ctx, orgID, dayOnly)
	if err != nil {
		return fmt.Errorf("analytics rollup: count messages: %w", err)
	}

	msgRows, deliveryRows := foldMessages(orgID, dayOnly, breakdowns)
	if err := s.repo.UpsertMessagesDaily(ctx, msgRows); err != nil {
		return fmt.Errorf("analytics rollup: upsert messages: %w", err)
	}
	if err := s.repo.UpsertDeliveryRateDaily(ctx, deliveryRows); err != nil {
		return fmt.Errorf("analytics rollup: upsert delivery: %w", err)
	}

	conv, err := s.raw.CountConversationsByDay(ctx, orgID, dayOnly)
	if err != nil {
		return fmt.Errorf("analytics rollup: count conversations: %w", err)
	}
	avg, err := s.raw.P50ResponseTimeByDay(ctx, orgID, dayOnly)
	if err != nil {
		return fmt.Errorf("analytics rollup: p50 response time: %w", err)
	}
	if err := s.repo.UpsertConversationsDaily(ctx, []danalytics.ConversationsDaily{{
		OrgID:                  orgID,
		Day:                    dayOnly,
		Opened:                 conv.Opened,
		Resolved:               conv.Resolved,
		AvgResponseTimeSeconds: avg,
	}}); err != nil {
		return fmt.Errorf("analytics rollup: upsert conversations: %w", err)
	}

	// Call rollups: per-direction detail rows + the pan-direction "all"
	// row are returned by the source in a single query, so we hand them
	// straight to the upsert.
	callRows, err := s.raw.CountCallsByDay(ctx, orgID, dayOnly)
	if err != nil {
		return fmt.Errorf("analytics rollup: count calls: %w", err)
	}
	if err := s.repo.UpsertCallsDaily(ctx, callRows); err != nil {
		return fmt.Errorf("analytics rollup: upsert calls: %w", err)
	}
	return nil
}

// Overview returns the compact dashboard aggregate over [from, to].
// The zero from or to is rejected as ErrInvalidRange — the caller must
// supply a bounded window.
func (s *Service) Overview(
	ctx context.Context,
	orgID organization.ID,
	from, to time.Time,
) (danalytics.Overview, error) {
	if from.IsZero() || to.IsZero() || from.After(to) {
		return danalytics.Overview{}, danalytics.ErrInvalidRange
	}
	msgs, err := s.repo.MessagesRange(ctx, orgID, from, to)
	if err != nil {
		return danalytics.Overview{}, fmt.Errorf("analytics overview: messages: %w", err)
	}
	convs, err := s.repo.ConversationsRange(ctx, orgID, from, to)
	if err != nil {
		return danalytics.Overview{}, fmt.Errorf("analytics overview: conversations: %w", err)
	}
	delivery, err := s.repo.DeliveryRateRange(ctx, orgID, from, to)
	if err != nil {
		return danalytics.Overview{}, fmt.Errorf("analytics overview: delivery: %w", err)
	}
	calls, err := s.repo.CallsRange(ctx, orgID, from, to)
	if err != nil {
		return danalytics.Overview{}, fmt.Errorf("analytics overview: calls: %w", err)
	}

	var total, opened int64
	for _, m := range msgs {
		// Total across the pan-provider + pan-type rows only, so we
		// don't double-count the per-provider / per-type detail rows.
		if m.Provider == providerAll && m.MessageType == typeAll {
			total += m.Total
		}
	}
	for _, c := range convs {
		opened += c.Opened
	}

	var sent, delivered int64
	for _, d := range delivery {
		if d.Provider != providerAll {
			continue
		}
		sent += d.Sent
		delivered += d.Delivered
	}
	rate := int64(0)
	if sent > 0 {
		rate = (delivered * 100) / sent
	}

	// p50 over the per-day averages: cheap enough since range is small.
	avgs := make([]int64, 0, len(convs))
	for _, c := range convs {
		if c.AvgResponseTimeSeconds > 0 {
			avgs = append(avgs, c.AvgResponseTimeSeconds)
		}
	}
	sort.Slice(avgs, func(i, j int) bool { return avgs[i] < avgs[j] })
	p50 := int64(0)
	if len(avgs) > 0 {
		p50 = avgs[len(avgs)/2]
	}

	// Calls overview: sum only the pan-direction "all" rows to avoid
	// double-counting the per-direction detail rows. If the rollup for
	// some days was written before the pan-direction "all" row was
	// backfilled (older deploys, or a partial rollup), fall back to
	// summing per-direction detail rows for those specific days so the
	// KPI card doesn't misleadingly show 0.
	var callsTotal, callsAnswered, callsDurTotal int64
	daysWithAll := map[time.Time]struct{}{}
	for _, c := range calls {
		if c.Direction != danalytics.CallDirectionAll {
			continue
		}
		daysWithAll[c.Day] = struct{}{}
		callsTotal += int64(c.Total)
		callsAnswered += int64(c.Answered)
		callsDurTotal += int64(c.DurationSecondsTotal)
	}
	for _, c := range calls {
		if c.Direction == danalytics.CallDirectionAll {
			continue
		}
		if _, ok := daysWithAll[c.Day]; ok {
			continue
		}
		callsTotal += int64(c.Total)
		callsAnswered += int64(c.Answered)
		callsDurTotal += int64(c.DurationSecondsTotal)
	}
	callsAvg := int64(0)
	if callsTotal > 0 {
		callsAvg = callsDurTotal / callsTotal
	}

	return danalytics.Overview{
		MessagesTotal:           total,
		DeliveryRatePct:         rate,
		ResponseTimeSecondsP50:  p50,
		ConversationsOpened:     opened,
		CallsTotal:              callsTotal,
		CallsAnswered:           callsAnswered,
		CallsAvgDurationSeconds: callsAvg,
	}, nil
}

// Series returns a labelled time series suitable for a line chart.
// Supported kinds are enumerated as SeriesKind constants; unknown
// values return ErrUnknownSeries.
//
// When provider is non-empty for SeriesMessagesDaily and
// SeriesDeliveryRate, only rows carrying that provider tag are folded
// into the series. Empty provider selects the pan-provider "all" row.
func (s *Service) Series(
	ctx context.Context,
	orgID organization.ID,
	kind danalytics.SeriesKind,
	from, to time.Time,
	provider string,
) (danalytics.Series, error) {
	if from.IsZero() || to.IsZero() || from.After(to) {
		return danalytics.Series{}, danalytics.ErrInvalidRange
	}
	prov := provider
	if prov == "" {
		prov = providerAll
	}

	switch kind {
	case danalytics.SeriesMessagesDaily:
		rows, err := s.repo.MessagesRange(ctx, orgID, from, to)
		if err != nil {
			return danalytics.Series{}, fmt.Errorf("analytics series messages: %w", err)
		}
		byDay := map[time.Time]int64{}
		for _, m := range rows {
			if m.Provider != prov || m.MessageType != typeAll {
				continue
			}
			byDay[m.Day] += m.Total
		}
		return danalytics.Series{Name: "messages", Points: pointsFromMap(byDay)}, nil

	case danalytics.SeriesDeliveryRate:
		rows, err := s.repo.DeliveryRateRange(ctx, orgID, from, to)
		if err != nil {
			return danalytics.Series{}, fmt.Errorf("analytics series delivery: %w", err)
		}
		byDay := map[time.Time]int64{}
		for _, d := range rows {
			if d.Provider != prov {
				continue
			}
			rate := int64(0)
			if d.Sent > 0 {
				rate = (d.Delivered * 100) / d.Sent
			}
			byDay[d.Day] = rate
		}
		return danalytics.Series{Name: "delivery_rate_pct", Points: pointsFromMap(byDay)}, nil

	case danalytics.SeriesConversationsOpened:
		rows, err := s.repo.ConversationsRange(ctx, orgID, from, to)
		if err != nil {
			return danalytics.Series{}, fmt.Errorf("analytics series conversations: %w", err)
		}
		byDay := map[time.Time]int64{}
		for _, c := range rows {
			byDay[c.Day] += c.Opened
		}
		return danalytics.Series{Name: "conversations_opened", Points: pointsFromMap(byDay)}, nil

	case danalytics.SeriesCallsDaily:
		rows, err := s.repo.CallsRange(ctx, orgID, from, to)
		if err != nil {
			return danalytics.Series{}, fmt.Errorf("analytics series calls: %w", err)
		}
		byDay := map[time.Time]int64{}
		for _, c := range rows {
			// Only fold the pan-direction "all" row so we don't
			// double-count the per-direction detail rows.
			if c.Direction != danalytics.CallDirectionAll {
				continue
			}
			byDay[c.Day] += int64(c.Total)
		}
		return danalytics.Series{Name: "calls", Points: pointsFromMap(byDay)}, nil
	}
	return danalytics.Series{}, danalytics.ErrUnknownSeries
}

// foldMessages folds raw per-(provider, direction, type) breakdowns
// into the rollup shape. It emits:
//   - one MessagesDaily row per exact (provider, direction, type) tuple
//     (detail row).
//   - one MessagesDaily row per direction with provider='all' AND
//     message_type='all' (grand-total row).
//   - one DeliveryRateDaily row per provider on outbound traffic, plus a
//     provider='all' aggregate.
//
// Aggregate rows are separate from detail rows so a query for the
// pan-provider total does not require an ONLINE SUM() over all rows.
func foldMessages(
	orgID organization.ID,
	day time.Time,
	breakdowns []repository.MessageDayBreakdown,
) ([]danalytics.MessagesDaily, []danalytics.DeliveryRateDaily) {
	msgs := make([]danalytics.MessagesDaily, 0, len(breakdowns)+2)
	// Detail rows.
	for _, b := range breakdowns {
		msgs = append(msgs, danalytics.MessagesDaily{
			OrgID:       orgID,
			Day:         day,
			Provider:    b.Provider,
			Direction:   b.Direction,
			MessageType: b.MessageType,
			Total:       b.Total,
			Delivered:   b.Delivered,
			ReadCount:   b.ReadCount,
			Failed:      b.Failed,
		})
	}
	// Pan-provider + pan-type aggregate rows per direction.
	agg := map[string]*danalytics.MessagesDaily{}
	for _, b := range breakdowns {
		row, ok := agg[b.Direction]
		if !ok {
			row = &danalytics.MessagesDaily{
				OrgID:       orgID,
				Day:         day,
				Provider:    providerAll,
				Direction:   b.Direction,
				MessageType: typeAll,
			}
			agg[b.Direction] = row
		}
		row.Total += b.Total
		row.Delivered += b.Delivered
		row.ReadCount += b.ReadCount
		row.Failed += b.Failed
	}
	for _, r := range agg {
		msgs = append(msgs, *r)
	}

	// Delivery-rate rows: only outbound traffic contributes to sent /
	// delivered. Group by provider, plus an 'all' aggregate.
	deliveryByProv := map[string]*danalytics.DeliveryRateDaily{}
	all := &danalytics.DeliveryRateDaily{
		OrgID:    orgID,
		Day:      day,
		Provider: providerAll,
	}
	deliveryByProv[providerAll] = all
	for _, b := range breakdowns {
		if b.Direction != "outbound" {
			continue
		}
		row, ok := deliveryByProv[b.Provider]
		if !ok {
			row = &danalytics.DeliveryRateDaily{
				OrgID:    orgID,
				Day:      day,
				Provider: b.Provider,
			}
			deliveryByProv[b.Provider] = row
		}
		// Sent = every message that entered the outbound pipeline: we
		// treat total as sent (queued messages are still "attempted
		// sends" from the operator's perspective and inflate the
		// denominator honestly).
		row.Sent += b.Total
		row.Delivered += b.Delivered
		row.ReadCount += b.ReadCount
		row.Failed += b.Failed
		if row != all {
			all.Sent += b.Total
			all.Delivered += b.Delivered
			all.ReadCount += b.ReadCount
			all.Failed += b.Failed
		}
	}
	delivery := make([]danalytics.DeliveryRateDaily, 0, len(deliveryByProv))
	for _, r := range deliveryByProv {
		delivery = append(delivery, *r)
	}
	return msgs, delivery
}

// pointsFromMap turns a day→value map into a Series.Points slice sorted
// by ascending day. Days with no data are simply absent — the frontend
// treats a gap as zero.
func pointsFromMap(byDay map[time.Time]int64) []Point {
	out := make([]Point, 0, len(byDay))
	for d, v := range byDay {
		out = append(out, Point{Day: d, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out
}

// Point is the local alias for danalytics.Point to keep the file
// self-contained without a fresh import at every call site.
type Point = danalytics.Point
