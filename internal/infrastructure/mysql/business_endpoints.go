package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// BusinessEndpoints implements repository.BusinessEndpointRepo.
type BusinessEndpoints struct {
	db *sql.DB
}

// NewBusinessEndpoints constructs a BusinessEndpoints repository.
func NewBusinessEndpoints(db *sql.DB) *BusinessEndpoints { return &BusinessEndpoints{db: db} }

// Get fetches an endpoint by (OrgID, ID).
func (b *BusinessEndpoints) Get(ctx context.Context, orgID organization.ID, id session.BusinessEndpointID) (repository.BusinessEndpoint, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints id: %w", err)
	}
	const q = `SELECT id, org_id, channel, provider, integration_id, external_id, display, metadata, created_at
	           FROM business_endpoints WHERE org_id = ? AND id = ? LIMIT 1`
	row := b.db.QueryRowContext(ctx, q, orgBytes, idBytes)
	return scanEndpoint(row.Scan)
}

// FindByExternalID resolves (org, provider, external_id) — the primary
// webhook-routing lookup.
func (b *BusinessEndpoints) FindByExternalID(ctx context.Context, orgID organization.ID, provider, externalID string) (repository.BusinessEndpoint, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints org: %w", err)
	}
	const q = `SELECT id, org_id, channel, provider, integration_id, external_id, display, metadata, created_at
	           FROM business_endpoints WHERE org_id = ? AND provider = ? AND external_id = ? LIMIT 1`
	row := b.db.QueryRowContext(ctx, q, orgBytes, provider, externalID)
	return scanEndpoint(row.Scan)
}

// List returns all endpoints for an org.
func (b *BusinessEndpoints) List(ctx context.Context, orgID organization.ID) ([]repository.BusinessEndpoint, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("endpoints org: %w", err)
	}
	const q = `SELECT id, org_id, channel, provider, integration_id, external_id, display, metadata, created_at
	           FROM business_endpoints WHERE org_id = ? ORDER BY created_at ASC`
	rows, err := b.db.QueryContext(ctx, q, orgBytes)
	if err != nil {
		return nil, fmt.Errorf("endpoints list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []repository.BusinessEndpoint
	for rows.Next() {
		ep, err := scanEndpoint(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("endpoints rows: %w", err)
	}
	return out, nil
}

// scanEndpoint decodes a single row into a BusinessEndpoint.
func scanEndpoint(scan func(dest ...any) error) (repository.BusinessEndpoint, error) {
	var (
		id, org, integ []byte
		channel        string
		provider       string
		externalID     string
		display        string
		metaBytes      []byte
		created        time.Time
	)
	if err := scan(&id, &org, &channel, &provider, &integ, &externalID, &display, &metaBytes, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.BusinessEndpoint{}, ErrNotFound
		}
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints bad org: %w", err)
	}
	integStr, err := ulidFromBytes(integ)
	if err != nil {
		return repository.BusinessEndpoint{}, fmt.Errorf("endpoints bad integration: %w", err)
	}
	var meta map[string]any
	if len(metaBytes) > 0 {
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			return repository.BusinessEndpoint{}, fmt.Errorf("endpoints metadata: %w", err)
		}
	}
	return repository.BusinessEndpoint{
		ID:            session.BusinessEndpointID(idStr),
		OrgID:         organization.ID(orgStr),
		Channel:       channel,
		Provider:      provider,
		IntegrationID: integStr,
		ExternalID:    externalID,
		Display:       display,
		Metadata:      meta,
		CreatedAt:     created,
	}, nil
}
