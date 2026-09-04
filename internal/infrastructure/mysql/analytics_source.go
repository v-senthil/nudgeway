package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/analytics"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// AnalyticsSource implements repository.AnalyticsSource by reading the
// canonical `messages` + `conversations` tables. It carries no
// provider-specific logic — every count is derived from the domain
// enums.
type AnalyticsSource struct {
	db *sql.DB
}

// NewAnalyticsSource constructs an AnalyticsSource against db.
func NewAnalyticsSource(db *sql.DB) *AnalyticsSource { return &AnalyticsSource{db: db} }

// CountMessagesByDay returns per-(provider, direction, type) breakdowns
// of the `messages` table for one UTC day. The status column is folded
// into three counters: delivered / read_count / failed. Total is the
// unconditional row count for the tuple.
func (a *AnalyticsSource) CountMessagesByDay(
	ctx context.Context,
	orgID organization.ID,
	day time.Time,
) ([]repository.MessageDayBreakdown, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("analytics source org: %w", err)
	}
	start := dateOf(day)
	end := start.Add(24 * time.Hour)
	const q = `SELECT
	    provider,
	    direction,
	    message_type,
	    COUNT(*)                                                            AS total,
	    SUM(CASE WHEN status IN ('delivered','read') THEN 1 ELSE 0 END)     AS delivered,
	    SUM(CASE WHEN status = 'read'                THEN 1 ELSE 0 END)     AS read_count,
	    SUM(CASE WHEN status = 'failed'              THEN 1 ELSE 0 END)     AS failed
	  FROM messages
	  WHERE org_id = ? AND created_at >= ? AND created_at < ?
	  GROUP BY provider, direction, message_type`
	rows, err := a.db.QueryContext(ctx, q, orgBytes, start, end)
	if err != nil {
		return nil, fmt.Errorf("analytics source messages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []repository.MessageDayBreakdown
	for rows.Next() {
		var b repository.MessageDayBreakdown
		if err := rows.Scan(&b.Provider, &b.Direction, &b.MessageType, &b.Total, &b.Delivered, &b.ReadCount, &b.Failed); err != nil {
			return nil, fmt.Errorf("analytics source messages scan: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics source messages rows: %w", err)
	}
	return out, nil
}

// CountConversationsByDay returns opened + resolved counters for one
// UTC day. "Opened" is derived from conversations.created_at falling in
// the day window; "resolved" from resolved_at.
func (a *AnalyticsSource) CountConversationsByDay(
	ctx context.Context,
	orgID organization.ID,
	day time.Time,
) (repository.ConversationDayCounts, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.ConversationDayCounts{}, fmt.Errorf("analytics source org: %w", err)
	}
	start := dateOf(day)
	end := start.Add(24 * time.Hour)
	const q = `SELECT
	    SUM(CASE WHEN created_at  >= ? AND created_at  < ? THEN 1 ELSE 0 END) AS opened,
	    SUM(CASE WHEN resolved_at >= ? AND resolved_at < ? THEN 1 ELSE 0 END) AS resolved
	  FROM conversations
	  WHERE org_id = ?
	    AND (
	      (created_at  >= ? AND created_at  < ?) OR
	      (resolved_at >= ? AND resolved_at < ?)
	    )`
	var opened, resolved sql.NullInt64
	err = a.db.QueryRowContext(ctx, q,
		start, end,
		start, end,
		orgBytes,
		start, end,
		start, end,
	).Scan(&opened, &resolved)
	if err != nil {
		return repository.ConversationDayCounts{}, fmt.Errorf("analytics source conversations: %w", err)
	}
	return repository.ConversationDayCounts{
		Opened:   opened.Int64,
		Resolved: resolved.Int64,
	}, nil
}

// P50ResponseTimeByDay returns a coarse average of seconds between an
// inbound message and the very next outbound message on the same
// conversation, sampled over the given UTC day.
//
// The self-join uses the (org_id, conversation_id, created_at) index on
// `messages` so scan cost stays bounded by the day's inbound message
// count. Returns zero when no inbound→outbound pairing exists.
func (a *AnalyticsSource) P50ResponseTimeByDay(
	ctx context.Context,
	orgID organization.ID,
	day time.Time,
) (int64, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return 0, fmt.Errorf("analytics source org: %w", err)
	}
	start := dateOf(day)
	end := start.Add(24 * time.Hour)
	// Coarse average of the elapsed seconds between each inbound
	// message and the next outbound message on the same conversation.
	// Using AVG keeps the query cheap; a true median would need
	// window functions the target MySQL 8 supports but we accept AVG
	// here — the domain shape is documented as coarse.
	const q = `SELECT COALESCE(AVG(TIMESTAMPDIFF(SECOND, m_in.created_at, m_out.created_at)), 0)
	  FROM messages m_in
	  JOIN messages m_out
	    ON m_out.org_id          = m_in.org_id
	   AND m_out.conversation_id = m_in.conversation_id
	   AND m_out.direction       = 'outbound'
	   AND m_out.created_at > m_in.created_at
	   AND m_out.created_at < ?
	   AND NOT EXISTS (
	     SELECT 1 FROM messages m_mid
	     WHERE m_mid.org_id          = m_in.org_id
	       AND m_mid.conversation_id = m_in.conversation_id
	       AND m_mid.direction       = 'outbound'
	       AND m_mid.created_at > m_in.created_at
	       AND m_mid.created_at < m_out.created_at
	   )
	  WHERE m_in.org_id     = ?
	    AND m_in.direction  = 'inbound'
	    AND m_in.created_at >= ?
	    AND m_in.created_at <  ?`
	var avg sql.NullFloat64
	// Widen the outbound cap by a day to catch replies that land after
	// midnight UTC on the target day.
	if err := a.db.QueryRowContext(ctx, q,
		end.Add(24*time.Hour),
		orgBytes,
		start,
		end,
	).Scan(&avg); err != nil {
		return 0, fmt.Errorf("analytics source p50: %w", err)
	}
	return int64(avg.Float64), nil
}

// CountCallsByDay returns per-direction breakdowns of the `calls` table
// for one UTC day. The status column is folded into the answered /
// completed / failed / missed counters. The pan-direction "all" aggregate
// row is included so downstream folding stays cheap.
func (a *AnalyticsSource) CountCallsByDay(
	ctx context.Context,
	orgID organization.ID,
	day time.Time,
) ([]analytics.CallsDaily, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("analytics source calls org: %w", err)
	}
	start := dateOf(day)
	end := start.Add(24 * time.Hour)
	const q = `SELECT
	    direction,
	    COUNT(*)                                                                    AS total,
	    SUM(CASE WHEN status IN ('answered','in_progress','completed') THEN 1 ELSE 0 END) AS answered,
	    SUM(CASE WHEN status = 'completed'                              THEN 1 ELSE 0 END) AS completed,
	    SUM(CASE WHEN status = 'failed'                                 THEN 1 ELSE 0 END) AS failed,
	    SUM(CASE WHEN status IN ('missed','no_answer')                  THEN 1 ELSE 0 END) AS missed,
	    COALESCE(SUM(duration_seconds), 0)                                          AS dur
	  FROM calls
	  WHERE org_id = ? AND created_at >= ? AND created_at < ?
	  GROUP BY direction`
	rows, err := a.db.QueryContext(ctx, q, orgBytes, start, end)
	if err != nil {
		return nil, fmt.Errorf("analytics source calls: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []analytics.CallsDaily{}
	// Aggregate the pan-direction "all" row inline as we scan.
	all := analytics.CallsDaily{
		OrgID:     orgID,
		Day:       start,
		Direction: analytics.CallDirectionAll,
	}
	for rows.Next() {
		var (
			direction string
			total     int
			answered  int
			completed int
			failed    int
			missed    int
			dur       int
		)
		if err := rows.Scan(&direction, &total, &answered, &completed, &failed, &missed, &dur); err != nil {
			return nil, fmt.Errorf("analytics source calls scan: %w", err)
		}
		out = append(out, analytics.CallsDaily{
			OrgID:                orgID,
			Day:                  start,
			Direction:            direction,
			Total:                total,
			Answered:             answered,
			Completed:            completed,
			Failed:               failed,
			Missed:               missed,
			DurationSecondsTotal: dur,
		})
		all.Total += total
		all.Answered += answered
		all.Completed += completed
		all.Failed += failed
		all.Missed += missed
		all.DurationSecondsTotal += dur
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("analytics source calls rows: %w", err)
	}
	out = append(out, all)
	return out, nil
}
