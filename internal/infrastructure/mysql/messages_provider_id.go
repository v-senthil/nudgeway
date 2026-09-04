package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// SetProviderMessageID stamps a message's provider_message_id after a
// successful outbound send. Called by the send worker with the wamid Meta
// returned. Idempotent: writing the same id twice is a no-op.
//
// Without this the outbound row's provider_message_id column stays NULL,
// which means later status callbacks (delivered / read) keyed on wamid
// can't find the row via UpdateStatusByProviderMessageID and the UI never
// advances past the single grey tick.
func (m *Messages) SetProviderMessageID(
	ctx context.Context,
	orgID organization.ID,
	id message.ID,
	providerMessageID string,
) error {
	if providerMessageID == "" {
		return nil
	}
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("messages set pmid: bad org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("messages set pmid: bad id: %w", err)
	}
	const q = `UPDATE messages
	              SET provider_message_id = ?
	            WHERE org_id = ? AND id = ?
	              AND (provider_message_id IS NULL OR provider_message_id = '')`
	res, err := m.db.ExecContext(ctx, q, providerMessageID, orgBytes, idBytes)
	if err != nil {
		return fmt.Errorf("messages set pmid exec: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("messages set pmid rows: %w", err)
	}
	if n == 0 {
		// Row already has a provider_message_id (idempotent replay) — not
		// an error. Distinguish real "row missing" via a follow-up count.
		var exists int
		countErr := m.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM messages WHERE org_id = ? AND id = ?`,
			orgBytes, idBytes,
		).Scan(&exists)
		if countErr != nil && !errors.Is(countErr, sql.ErrNoRows) {
			return fmt.Errorf("messages set pmid recount: %w", countErr)
		}
		if exists == 0 {
			return fmt.Errorf("messages set pmid: row not found")
		}
	}
	return nil
}
