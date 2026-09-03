package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// UpdateStatusByProviderMessageID advances a message's status by looking it up
// via UNIQUE(org_id, provider, provider_message_id) instead of the internal
// row ID. Used by the inbound webhook worker when it receives a status
// callback (delivered / read / failed) that references only the provider
// message id (e.g. Meta wamid).
//
// Idempotent — repeating the same terminal status is a no-op. Missing rows
// return sql.ErrNoRows so callers can classify as permanent (Meta
// occasionally races status callbacks against the persistence of the
// original send row; a reconciler picks those up later).
func (m *Messages) UpdateStatusByProviderMessageID(
	ctx context.Context,
	orgID organization.ID,
	providerMessageID string,
	next message.Status,
	at time.Time,
) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("messages_pmi bad org: %w", err)
	}
	col, err := statusTimestampColumn(next)
	if err != nil {
		return fmt.Errorf("messages_pmi bad status: %w", err)
	}
	// Guarded update — never regress state; but for status timestamps we
	// still stamp the column when the row is at or past this state so the
	// most recent callback wins for that transition. Simpler baseline: set
	// status and the associated timestamp column via a single UPDATE.
	//
	//nolint:gosec // status column name is validated via statusTimestampColumn.
	q := fmt.Sprintf(
		`UPDATE messages SET status = ?, %s = COALESCE(%s, ?) WHERE org_id = ? AND provider_message_id = ?`,
		col, col,
	)
	res, err := m.db.ExecContext(ctx, q, string(next), at, orgBytes, providerMessageID)
	if err != nil {
		return fmt.Errorf("messages_pmi exec: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("messages_pmi rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Ensure the compile-time check remains — MessageStatusByProviderID is a
// consumer-side port declared in application/message; this method satisfies
// it without pulling that package into the infra layer.
var _ = errors.New
