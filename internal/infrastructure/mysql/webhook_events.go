package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// WebhookEvents implements repository.WebhookEventRepo against the
// webhook_events table.
type WebhookEvents struct {
	db *sql.DB
}

// NewWebhookEvents constructs a WebhookEvents repository.
func NewWebhookEvents(db *sql.DB) *WebhookEvents { return &WebhookEvents{db: db} }

// Insert stores a WebhookEvent. Returns created=true on a real insert and
// created=false with nil error on UNIQUE(integration_id, external_event_id)
// duplicates so callers can absorb duplicate deliveries as no-ops.
func (r *WebhookEvents) Insert(ctx context.Context, evt integration.WebhookEvent) (bool, error) {
	// Mint an ID when the caller left it empty — webhook events are opaque
	// bookkeeping rows; the ingress helper does not need to allocate its own.
	if evt.ID == "" {
		evt.ID = integration.WebhookEventID(ulid.Make().String())
	}
	idBytes, err := ulidToBytes(string(evt.ID))
	if err != nil {
		return false, fmt.Errorf("webhook_events id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(evt.OrgID))
	if err != nil {
		return false, fmt.Errorf("webhook_events org: %w", err)
	}
	integBytes, err := ulidToBytes(string(evt.IntegrationID))
	if err != nil {
		return false, fmt.Errorf("webhook_events integration: %w", err)
	}
	status := string(evt.Status)
	if status == "" {
		status = string(integration.WebhookStatusReceived)
	}
	var errMsg any
	if evt.Error != "" {
		errMsg = evt.Error
	}
	var rawBody any
	if len(evt.RawBody) > 0 {
		rawBody = evt.RawBody
	}
	const q = `INSERT INTO webhook_events
	    (id, org_id, integration_id, provider, external_event_id, status, raw_ref, raw_body, error)
	  VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`
	_, err = r.db.ExecContext(ctx, q,
		idBytes, orgBytes, integBytes, evt.Provider, evt.ExternalEventID, status, rawBody, errMsg,
	)
	if err != nil {
		if isDuplicateErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("webhook_events insert: %w", err)
	}
	return true, nil
}

// MarkProcessed transitions the row to Status=processed.
func (r *WebhookEvents) MarkProcessed(ctx context.Context, id integration.WebhookEventID) error {
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("webhook_events id: %w", err)
	}
	const q = `UPDATE webhook_events SET status = 'processed', processed_at = ?, error = NULL WHERE id = ?`
	res, err := r.db.ExecContext(ctx, q, time.Now().UTC(), idBytes)
	if err != nil {
		return fmt.Errorf("webhook_events processed: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkFailed transitions the row to Status=failed with the given error.
func (r *WebhookEvents) MarkFailed(ctx context.Context, id integration.WebhookEventID, errMsg string) error {
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("webhook_events id: %w", err)
	}
	const q = `UPDATE webhook_events SET status = 'failed', processed_at = ?, error = ? WHERE id = ?`
	res, err := r.db.ExecContext(ctx, q, time.Now().UTC(), errMsg, idBytes)
	if err != nil {
		return fmt.Errorf("webhook_events failed: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Get fetches a WebhookEvent by ID.
func (r *WebhookEvents) Get(ctx context.Context, id integration.WebhookEventID) (integration.WebhookEvent, error) {
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return integration.WebhookEvent{}, fmt.Errorf("webhook_events id: %w", err)
	}
	const q = `SELECT id, org_id, integration_id, provider, external_event_id, received_at, processed_at, status, raw_body, error
	           FROM webhook_events WHERE id = ? LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, idBytes)
	return scanWebhookEvent(row.Scan)
}

// ListPending returns up to `limit` rows still in Status=received,
// oldest first.
func (r *WebhookEvents) ListPending(ctx context.Context, limit int) ([]integration.WebhookEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const q = `SELECT id, org_id, integration_id, provider, external_event_id, received_at, processed_at, status, raw_body, error
	           FROM webhook_events WHERE status = 'received' ORDER BY received_at ASC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("webhook_events pending: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []integration.WebhookEvent
	for rows.Next() {
		evt, err := scanWebhookEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, evt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("webhook_events rows: %w", err)
	}
	return out, nil
}

// scanWebhookEvent decodes a row into integration.WebhookEvent.
func scanWebhookEvent(scan func(dest ...any) error) (integration.WebhookEvent, error) {
	var (
		id, org, integ  []byte
		provider, ext   string
		received        time.Time
		processed       sql.NullTime
		status          string
		rawBody         []byte
		errMsg          sql.NullString
	)
	if err := scan(&id, &org, &integ, &provider, &ext, &received, &processed, &status, &rawBody, &errMsg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return integration.WebhookEvent{}, ErrNotFound
		}
		return integration.WebhookEvent{}, fmt.Errorf("webhook_events scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return integration.WebhookEvent{}, fmt.Errorf("webhook_events bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return integration.WebhookEvent{}, fmt.Errorf("webhook_events bad org: %w", err)
	}
	integStr, err := ulidFromBytes(integ)
	if err != nil {
		return integration.WebhookEvent{}, fmt.Errorf("webhook_events bad integration: %w", err)
	}
	out := integration.WebhookEvent{
		ID:              integration.WebhookEventID(idStr),
		OrgID:           organization.ID(orgStr),
		IntegrationID:   integration.ID(integStr),
		Provider:        provider,
		ExternalEventID: ext,
		ReceivedAt:      received,
		Status:          integration.WebhookEventStatus(status),
		RawBody:         append([]byte(nil), rawBody...),
	}
	if processed.Valid {
		t := processed.Time
		out.ProcessedAt = &t
	}
	if errMsg.Valid {
		out.Error = errMsg.String
	}
	return out, nil
}
