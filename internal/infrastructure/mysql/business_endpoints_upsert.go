package mysql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/v-senthil/nudgeway/internal/domain/session"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// Upsert finds-or-creates the (org, provider, external_id) tuple, linking
// it to the given integration. Used by the Integrations application
// service when the operator provisions a channel-kind integration.
//
// Idempotent: on conflict, the row's integration_id + display are updated
// (channel + provider + external_id are immutable identity keys). Returns
// the row's canonical BusinessEndpointID.
func (b *BusinessEndpoints) Upsert(ctx context.Context, ep repository.BusinessEndpoint) (session.BusinessEndpointID, error) {
	orgBytes, err := ulidToBytes(string(ep.OrgID))
	if err != nil {
		return "", fmt.Errorf("endpoints upsert bad org: %w", err)
	}
	integrationBytes, err := ulidToBytes(ep.IntegrationID)
	if err != nil {
		return "", fmt.Errorf("endpoints upsert bad integration id: %w", err)
	}
	metaJSON, err := json.Marshal(ep.Metadata)
	if err != nil {
		return "", fmt.Errorf("endpoints upsert marshal metadata: %w", err)
	}

	// Try to find an existing row first so we can return its ID unchanged.
	const findQ = `SELECT id FROM business_endpoints WHERE org_id = ? AND provider = ? AND external_id = ? LIMIT 1`
	var existing []byte
	err = b.db.QueryRowContext(ctx, findQ, orgBytes, ep.Provider, ep.ExternalID).Scan(&existing)
	if err == nil {
		// Update mutable fields.
		const updQ = `UPDATE business_endpoints SET integration_id = ?, display = ?, metadata = ? WHERE id = ?`
		if _, err := b.db.ExecContext(ctx, updQ, integrationBytes, ep.Display, metaJSON, existing); err != nil {
			return "", fmt.Errorf("endpoints upsert update: %w", err)
		}
		s, err := ulidFromBytes(existing)
		if err != nil {
			return "", fmt.Errorf("endpoints upsert bad existing id: %w", err)
		}
		return session.BusinessEndpointID(s), nil
	}

	// Insert new row.
	newID := ulid.Make()
	const insQ = `INSERT INTO business_endpoints (id, org_id, channel, provider, integration_id, external_id, display, metadata)
	              VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := b.db.ExecContext(ctx, insQ,
		newID[:], orgBytes, ep.Channel, ep.Provider, integrationBytes,
		ep.ExternalID, ep.Display, metaJSON,
	); err != nil {
		return "", fmt.Errorf("endpoints upsert insert: %w", err)
	}
	return session.BusinessEndpointID(newID.String()), nil
}
