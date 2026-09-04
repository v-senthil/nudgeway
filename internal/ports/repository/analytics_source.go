package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/analytics"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// MessageDayBreakdown is one row returned by
// AnalyticsSource.CountMessagesByDay — a per-(provider, direction,
// message_type) tuple with the raw counters aggregated over the day.
type MessageDayBreakdown struct {
	// Provider is the registry key of the provider that carried the
	// message ("whatsapp", ...).
	Provider string
	// Direction is "inbound" or "outbound".
	Direction string
	// MessageType is the domain message type (e.g. "text", "image").
	MessageType string
	// Total is the count of matching messages on the day.
	Total int64
	// Delivered is the subset in the "delivered" state or later.
	Delivered int64
	// ReadCount is the subset in the "read" state.
	ReadCount int64
	// Failed is the subset in the "failed" state.
	Failed int64
}

// ConversationDayCounts is the raw conversation counters for a single
// UTC day: how many conversations were created and how many transitioned
// into the resolved state that day.
type ConversationDayCounts struct {
	// Opened is the number of conversations created on the day (i.e.
	// their `created_at` falls in the UTC 00:00..24:00 window).
	Opened int64
	// Resolved is the number of conversations whose `resolved_at` falls
	// in the day window.
	Resolved int64
}

// AnalyticsSource reads raw canonical tables (`messages`,
// `conversations`) to feed the rollup pipeline. It is a read-only view
// — every method scopes by (org_id, day) so multi-tenant isolation is
// preserved at the query layer.
type AnalyticsSource interface {
	// CountMessagesByDay returns per-(provider, direction, type)
	// breakdowns of the `messages` table for the given UTC day.
	CountMessagesByDay(ctx context.Context, orgID organization.ID, day time.Time) ([]MessageDayBreakdown, error)

	// CountConversationsByDay returns the raw opened/resolved counters
	// for the given UTC day.
	CountConversationsByDay(ctx context.Context, orgID organization.ID, day time.Time) (ConversationDayCounts, error)

	// P50ResponseTimeByDay returns a coarse average of seconds between
	// an inbound message and the next outbound message on the same
	// conversation, sampled over the given day. Returns zero when the
	// day has no such pairings.
	P50ResponseTimeByDay(ctx context.Context, orgID organization.ID, day time.Time) (int64, error)

	// CountCallsByDay returns per-direction breakdowns of the `calls`
	// table for the given UTC day. Implementations MUST also return a
	// pan-direction "all" row (direction = analytics.CallDirectionAll)
	// so downstream folding stays cheap.
	CountCallsByDay(ctx context.Context, orgID organization.ID, day time.Time) ([]analytics.CallsDaily, error)
}
