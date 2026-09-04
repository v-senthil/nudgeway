package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fullwa/fullwa/internal/domain/analytics"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// Analytics implements repository.AnalyticsRepo against MySQL. Rows land
// in the analytics_* tables declared in
// migrations/20260904000006_analytics_rollups.up.sql.
type Analytics struct {
	db *sql.DB
}

// NewAnalytics constructs an Analytics repository against db.
func NewAnalytics(db *sql.DB) *Analytics { return &Analytics{db: db} }

// UpsertMessagesDaily inserts or updates analytics_messages_daily rows.
// Each row's composite PK (org_id, day, provider, direction, message_type)
// drives the ON DUPLICATE KEY UPDATE clause — running Rollup twice for
// the same day produces the same table state.
func (a *Analytics) UpsertMessagesDaily(
	ctx context.Context,
	entries []analytics.MessagesDaily,
) error {
	if len(entries) == 0 {
		return nil
	}
	// Batch into one INSERT to save round-trips. The tuple count is
	// small (worst-case tens of rows per day per org).
	placeholders := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*9)
	for _, e := range entries {
		orgBytes, err := ulidToBytes(string(e.OrgID))
		if err != nil {
			return fmt.Errorf("analytics messages org: %w", err)
		}
		placeholders = append(placeholders, "(?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			orgBytes,
			e.Day.UTC(),
			e.Provider,
			e.Direction,
			e.MessageType,
			e.Total,
			e.Delivered,
			e.ReadCount,
			e.Failed,
		)
	}
	q := `INSERT INTO analytics_messages_daily
	    (org_id, day, provider, direction, message_type, total, delivered, read_count, failed)
	  VALUES ` + strings.Join(placeholders, ", ") + `
	  ON DUPLICATE KEY UPDATE
	    total      = VALUES(total),
	    delivered  = VALUES(delivered),
	    read_count = VALUES(read_count),
	    failed     = VALUES(failed)`
	if _, err := a.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("analytics messages upsert: %w", err)
	}
	return nil
}

// UpsertConversationsDaily inserts or updates analytics_conversations_daily.
func (a *Analytics) UpsertConversationsDaily(
	ctx context.Context,
	entries []analytics.ConversationsDaily,
) error {
	if len(entries) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*5)
	for _, e := range entries {
		orgBytes, err := ulidToBytes(string(e.OrgID))
		if err != nil {
			return fmt.Errorf("analytics conversations org: %w", err)
		}
		placeholders = append(placeholders, "(?, ?, ?, ?, ?)")
		args = append(args,
			orgBytes,
			e.Day.UTC(),
			e.Opened,
			e.Resolved,
			e.AvgResponseTimeSeconds,
		)
	}
	q := `INSERT INTO analytics_conversations_daily
	    (org_id, day, opened, resolved, avg_response_time_seconds)
	  VALUES ` + strings.Join(placeholders, ", ") + `
	  ON DUPLICATE KEY UPDATE
	    opened                    = VALUES(opened),
	    resolved                  = VALUES(resolved),
	    avg_response_time_seconds = VALUES(avg_response_time_seconds)`
	if _, err := a.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("analytics conversations upsert: %w", err)
	}
	return nil
}

// UpsertDeliveryRateDaily inserts or updates analytics_delivery_rate_daily.
func (a *Analytics) UpsertDeliveryRateDaily(
	ctx context.Context,
	entries []analytics.DeliveryRateDaily,
) error {
	if len(entries) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*7)
	for _, e := range entries {
		orgBytes, err := ulidToBytes(string(e.OrgID))
		if err != nil {
			return fmt.Errorf("analytics delivery org: %w", err)
		}
		placeholders = append(placeholders, "(?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			orgBytes,
			e.Day.UTC(),
			e.Provider,
			e.Sent,
			e.Delivered,
			e.ReadCount,
			e.Failed,
		)
	}
	q := `INSERT INTO analytics_delivery_rate_daily
	    (org_id, day, provider, sent, delivered, read_count, failed)
	  VALUES ` + strings.Join(placeholders, ", ") + `
	  ON DUPLICATE KEY UPDATE
	    sent       = VALUES(sent),
	    delivered  = VALUES(delivered),
	    read_count = VALUES(read_count),
	    failed     = VALUES(failed)`
	if _, err := a.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("analytics delivery upsert: %w", err)
	}
	return nil
}

// MessagesRange returns rows for the org where day is in [from, to].
func (a *Analytics) MessagesRange(
	ctx context.Context,
	orgID organization.ID,
	from, to time.Time,
) ([]analytics.MessagesDaily, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("analytics messages org: %w", err)
	}
	const q = `SELECT day, provider, direction, message_type, total, delivered, read_count, failed
	  FROM analytics_messages_daily
	  WHERE org_id = ? AND day BETWEEN ? AND ?
	  ORDER BY day ASC`
	rows, err := a.db.QueryContext(ctx, q, orgBytes, dateOf(from), dateOf(to))
	if err != nil {
		return nil, fmt.Errorf("analytics messages range: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []analytics.MessagesDaily
	for rows.Next() {
		var (
			day       time.Time
			provider  string
			direction string
			mtype     string
			total     int64
			delivered int64
			readCount int64
			failed    int64
		)
		if err := rows.Scan(&day, &provider, &direction, &mtype, &total, &delivered, &readCount, &failed); err != nil {
			return nil, fmt.Errorf("analytics messages scan: %w", err)
		}
		out = append(out, analytics.MessagesDaily{
			OrgID:       orgID,
			Day:         day.UTC(),
			Provider:    provider,
			Direction:   direction,
			MessageType: mtype,
			Total:       total,
			Delivered:   delivered,
			ReadCount:   readCount,
			Failed:      failed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics messages rows: %w", err)
	}
	return out, nil
}

// ConversationsRange returns rows for the org where day is in [from, to].
func (a *Analytics) ConversationsRange(
	ctx context.Context,
	orgID organization.ID,
	from, to time.Time,
) ([]analytics.ConversationsDaily, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("analytics conversations org: %w", err)
	}
	const q = `SELECT day, opened, resolved, avg_response_time_seconds
	  FROM analytics_conversations_daily
	  WHERE org_id = ? AND day BETWEEN ? AND ?
	  ORDER BY day ASC`
	rows, err := a.db.QueryContext(ctx, q, orgBytes, dateOf(from), dateOf(to))
	if err != nil {
		return nil, fmt.Errorf("analytics conversations range: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []analytics.ConversationsDaily
	for rows.Next() {
		var (
			day      time.Time
			opened   int64
			resolved int64
			avg      int64
		)
		if err := rows.Scan(&day, &opened, &resolved, &avg); err != nil {
			return nil, fmt.Errorf("analytics conversations scan: %w", err)
		}
		out = append(out, analytics.ConversationsDaily{
			OrgID:                  orgID,
			Day:                    day.UTC(),
			Opened:                 opened,
			Resolved:               resolved,
			AvgResponseTimeSeconds: avg,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics conversations rows: %w", err)
	}
	return out, nil
}

// DeliveryRateRange returns rows for the org where day is in [from, to].
func (a *Analytics) DeliveryRateRange(
	ctx context.Context,
	orgID organization.ID,
	from, to time.Time,
) ([]analytics.DeliveryRateDaily, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("analytics delivery org: %w", err)
	}
	const q = `SELECT day, provider, sent, delivered, read_count, failed
	  FROM analytics_delivery_rate_daily
	  WHERE org_id = ? AND day BETWEEN ? AND ?
	  ORDER BY day ASC`
	rows, err := a.db.QueryContext(ctx, q, orgBytes, dateOf(from), dateOf(to))
	if err != nil {
		return nil, fmt.Errorf("analytics delivery range: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []analytics.DeliveryRateDaily
	for rows.Next() {
		var (
			day       time.Time
			provider  string
			sent      int64
			delivered int64
			readCount int64
			failed    int64
		)
		if err := rows.Scan(&day, &provider, &sent, &delivered, &readCount, &failed); err != nil {
			return nil, fmt.Errorf("analytics delivery scan: %w", err)
		}
		out = append(out, analytics.DeliveryRateDaily{
			OrgID:     orgID,
			Day:       day.UTC(),
			Provider:  provider,
			Sent:      sent,
			Delivered: delivered,
			ReadCount: readCount,
			Failed:    failed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics delivery rows: %w", err)
	}
	return out, nil
}

// RollupState returns the last day the named job successfully rolled
// up, or the zero time if the bookmark row is absent.
func (a *Analytics) RollupState(
	ctx context.Context,
	jobName string,
) (time.Time, error) {
	const q = `SELECT last_processed_day FROM analytics_rollup_state WHERE job_name = ?`
	var day sql.NullTime
	err := a.db.QueryRowContext(ctx, q, jobName).Scan(&day)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("analytics state get: %w", err)
	}
	if !day.Valid {
		return time.Time{}, nil
	}
	return day.Time.UTC(), nil
}

// SaveRollupState upserts the bookmark row for the named job.
func (a *Analytics) SaveRollupState(
	ctx context.Context,
	jobName string,
	lastDay time.Time,
) error {
	const q = `INSERT INTO analytics_rollup_state (job_name, last_processed_day, last_ran_at)
	  VALUES (?, ?, ?)
	  ON DUPLICATE KEY UPDATE
	    last_processed_day = VALUES(last_processed_day),
	    last_ran_at        = VALUES(last_ran_at)`
	var day any
	if !lastDay.IsZero() {
		day = dateOf(lastDay)
	}
	_, err := a.db.ExecContext(ctx, q, jobName, day, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("analytics state save: %w", err)
	}
	return nil
}

// UpsertCallsDaily inserts or updates analytics_calls_daily rows. The
// composite (org_id, day, direction) PK drives the ON DUPLICATE KEY
// UPDATE clause — running Rollup twice for the same day produces the
// same table state.
func (a *Analytics) UpsertCallsDaily(
	ctx context.Context,
	entries []analytics.CallsDaily,
) error {
	if len(entries) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*9)
	for _, e := range entries {
		orgBytes, err := ulidToBytes(string(e.OrgID))
		if err != nil {
			return fmt.Errorf("analytics calls org: %w", err)
		}
		placeholders = append(placeholders, "(?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			orgBytes,
			e.Day.UTC(),
			e.Direction,
			e.Total,
			e.Answered,
			e.Completed,
			e.Failed,
			e.Missed,
			e.DurationSecondsTotal,
		)
	}
	q := `INSERT INTO analytics_calls_daily
	    (org_id, day, direction, total, answered, completed, failed, missed, duration_seconds_total)
	  VALUES ` + strings.Join(placeholders, ", ") + `
	  ON DUPLICATE KEY UPDATE
	    total                  = VALUES(total),
	    answered               = VALUES(answered),
	    completed              = VALUES(completed),
	    failed                 = VALUES(failed),
	    missed                 = VALUES(missed),
	    duration_seconds_total = VALUES(duration_seconds_total)`
	if _, err := a.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("analytics calls upsert: %w", err)
	}
	return nil
}

// CallsRange returns rows for the org where day is in [from, to].
func (a *Analytics) CallsRange(
	ctx context.Context,
	orgID organization.ID,
	from, to time.Time,
) ([]analytics.CallsDaily, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("analytics calls org: %w", err)
	}
	const q = `SELECT day, direction, total, answered, completed, failed, missed, duration_seconds_total
	  FROM analytics_calls_daily
	  WHERE org_id = ? AND day BETWEEN ? AND ?
	  ORDER BY day ASC`
	rows, err := a.db.QueryContext(ctx, q, orgBytes, dateOf(from), dateOf(to))
	if err != nil {
		return nil, fmt.Errorf("analytics calls range: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []analytics.CallsDaily
	for rows.Next() {
		var (
			day       time.Time
			direction string
			total     int
			answered  int
			completed int
			failed    int
			missed    int
			durTotal  int
		)
		if err := rows.Scan(&day, &direction, &total, &answered, &completed, &failed, &missed, &durTotal); err != nil {
			return nil, fmt.Errorf("analytics calls scan: %w", err)
		}
		out = append(out, analytics.CallsDaily{
			OrgID:                orgID,
			Day:                  day.UTC(),
			Direction:            direction,
			Total:                total,
			Answered:             answered,
			Completed:            completed,
			Failed:               failed,
			Missed:               missed,
			DurationSecondsTotal: durTotal,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics calls rows: %w", err)
	}
	return out, nil
}

// dateOf strips the time-of-day component and pins the value to UTC.
// MySQL DATE columns silently truncate, but explicit is safer for the
// occasional driver that round-trips the value as a DATETIME.
func dateOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
