package repository

import (
	"context"
	"time"

	"github.com/fullwa/fullwa/internal/domain/analytics"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// AnalyticsRepo persists and reads the daily rollup tables. All writes
// are upserts keyed on the composite primary key of each table, so the
// worker can safely re-run a day without accumulating duplicates.
//
// Read methods take a UTC [from, to] inclusive-inclusive range on Day
// and return rows ordered by ascending Day.
type AnalyticsRepo interface {
	// UpsertMessagesDaily persists a batch of message rollup rows.
	// The composite (org_id, day, provider, direction, message_type)
	// PK drives an ON DUPLICATE KEY UPDATE — running Rollup twice for
	// the same day produces the same table state.
	UpsertMessagesDaily(ctx context.Context, entries []analytics.MessagesDaily) error

	// UpsertConversationsDaily persists conversation rollup rows.
	UpsertConversationsDaily(ctx context.Context, entries []analytics.ConversationsDaily) error

	// UpsertDeliveryRateDaily persists delivery-rate rollup rows.
	UpsertDeliveryRateDaily(ctx context.Context, entries []analytics.DeliveryRateDaily) error

	// MessagesRange returns all message rollup rows for the org where
	// day is in [from, to] (inclusive).
	MessagesRange(ctx context.Context, orgID organization.ID, from, to time.Time) ([]analytics.MessagesDaily, error)

	// ConversationsRange returns conversation rollup rows for the org.
	ConversationsRange(ctx context.Context, orgID organization.ID, from, to time.Time) ([]analytics.ConversationsDaily, error)

	// DeliveryRateRange returns delivery-rate rollup rows for the org.
	DeliveryRateRange(ctx context.Context, orgID organization.ID, from, to time.Time) ([]analytics.DeliveryRateDaily, error)

	// RollupState returns the last day the named job successfully
	// rolled up, or the zero time if no state is persisted yet.
	RollupState(ctx context.Context, jobName string) (lastDay time.Time, err error)

	// SaveRollupState persists lastDay against the job's bookmark row,
	// stamping last_ran_at with the current UTC time.
	SaveRollupState(ctx context.Context, jobName string, lastDay time.Time) error

	// UpsertCallsDaily persists a batch of call rollup rows. The composite
	// (org_id, day, direction) PK drives an ON DUPLICATE KEY UPDATE.
	UpsertCallsDaily(ctx context.Context, entries []analytics.CallsDaily) error

	// CallsRange returns all call rollup rows for the org where day is in
	// [from, to] (inclusive).
	CallsRange(ctx context.Context, orgID organization.ID, from, to time.Time) ([]analytics.CallsDaily, error)
}
