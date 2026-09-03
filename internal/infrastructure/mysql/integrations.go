package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/infrastructure/crypto"
)

// Integrations implements repository.IntegrationRepo against the
// integrations + integration_credentials tables. The envelope handles all
// secret material; non-secret configuration lives in JSON columns.
type Integrations struct {
	db  *sql.DB
	env *crypto.Envelope
}

// NewIntegrations constructs an Integrations repository. env is required
// only when secret decryption is exercised (GetWithSecrets, EnsureIntegration).
// Read-only callers may pass nil.
func NewIntegrations(db *sql.DB, env *crypto.Envelope) *Integrations {
	return &Integrations{db: db, env: env}
}

// integrationStatusToDB collapses the expressive domain-status vocabulary
// onto the narrower ENUM stored in MySQL.
func integrationStatusToDB(s integration.Status) string {
	switch s {
	case integration.StatusConnected:
		return "active"
	case integration.StatusDisconnected:
		return "disabled"
	case integration.StatusAuthFailed, integration.StatusRateLimited, integration.StatusDegraded:
		return "error"
	case "":
		return "pending"
	default:
		return "pending"
	}
}

// integrationStatusFromDB expands the stored ENUM back into the domain
// vocabulary. "error" collapses to Degraded — callers that need finer
// distinctions read Health.
func integrationStatusFromDB(s string) integration.Status {
	switch s {
	case "active":
		return integration.StatusConnected
	case "disabled":
		return integration.StatusDisconnected
	case "error":
		return integration.StatusDegraded
	case "pending":
		return integration.StatusConnected // treat pending as connected pre-verification
	default:
		return integration.StatusConnected
	}
}

// Get fetches an Integration by (OrgID, ID).
func (r *Integrations) Get(ctx context.Context, orgID organization.ID, id integration.ID) (integration.Integration, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return integration.Integration{}, fmt.Errorf("integrations org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return integration.Integration{}, fmt.Errorf("integrations id: %w", err)
	}
	const q = `SELECT id, org_id, type, provider, name, status, config, credentials_ref, capabilities, health, created_at, updated_at
	           FROM integrations WHERE org_id = ? AND id = ? LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, orgBytes, idBytes)
	return scanIntegration(row.Scan)
}

// List returns every Integration owned by the org, ordered by CreatedAt.
func (r *Integrations) List(ctx context.Context, orgID organization.ID) ([]integration.Integration, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("integrations org: %w", err)
	}
	const q = `SELECT id, org_id, type, provider, name, status, config, credentials_ref, capabilities, health, created_at, updated_at
	           FROM integrations WHERE org_id = ? ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, q, orgBytes)
	if err != nil {
		return nil, fmt.Errorf("integrations list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []integration.Integration
	for rows.Next() {
		got, err := scanIntegration(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("integrations rows: %w", err)
	}
	return out, nil
}

// Create inserts a new Integration row.
func (r *Integrations) Create(ctx context.Context, i integration.Integration) error {
	idBytes, err := ulidToBytes(string(i.ID))
	if err != nil {
		return fmt.Errorf("integrations id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(i.OrgID))
	if err != nil {
		return fmt.Errorf("integrations org: %w", err)
	}
	cfg := jsonMustMarshal(orEmptyMap(i.Config))
	caps := jsonMustMarshal(orEmptyBoolMap(i.Capabilities))
	hlth := jsonMustMarshal(orEmptyMap(i.Health))
	var credRef any
	if len(i.CredentialsRef) > 0 {
		credRef = i.CredentialsRef
	}
	const q = `INSERT INTO integrations
	    (id, org_id, type, provider, name, status, config, credentials_ref, capabilities, health)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, q,
		idBytes, orgBytes, string(i.Type), i.Provider, i.Name,
		integrationStatusToDB(i.Status), cfg, credRef, caps, hlth,
	); err != nil {
		return fmt.Errorf("integrations insert: %w", err)
	}
	return nil
}

// Update persists mutable fields of an Integration.
func (r *Integrations) Update(ctx context.Context, i integration.Integration) error {
	idBytes, err := ulidToBytes(string(i.ID))
	if err != nil {
		return fmt.Errorf("integrations id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(i.OrgID))
	if err != nil {
		return fmt.Errorf("integrations org: %w", err)
	}
	cfg := jsonMustMarshal(orEmptyMap(i.Config))
	caps := jsonMustMarshal(orEmptyBoolMap(i.Capabilities))
	hlth := jsonMustMarshal(orEmptyMap(i.Health))
	const q = `UPDATE integrations
	           SET name = ?, status = ?, config = ?, capabilities = ?, health = ?
	           WHERE org_id = ? AND id = ?`
	res, err := r.db.ExecContext(ctx, q, i.Name, integrationStatusToDB(i.Status), cfg, caps, hlth, orgBytes, idBytes)
	if err != nil {
		return fmt.Errorf("integrations update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an Integration row. Any associated integration_credentials
// row is deleted first so the FK constraint holds.
func (r *Integrations) Delete(ctx context.Context, orgID organization.ID, id integration.ID) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("integrations org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("integrations id: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("integrations tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM integration_credentials WHERE org_id = ? AND integration_id = ?`, orgBytes, idBytes); err != nil {
		return fmt.Errorf("integrations delete credentials: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM integrations WHERE org_id = ? AND id = ?`, orgBytes, idBytes)
	if err != nil {
		return fmt.Errorf("integrations delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("integrations delete commit: %w", err)
	}
	return nil
}

// GetWithSecrets returns the Integration along with its decrypted secret
// map (secret name → value). The map is empty when no
// integration_credentials row exists. Requires an Envelope on the
// repository.
//
// When orgID is empty, the lookup falls back to the id-only PK path — used
// by the webhook ingress, which only has an integration_id from the URL
// and derives the org from the returned row.
func (r *Integrations) GetWithSecrets(ctx context.Context, orgID organization.ID, id integration.ID) (integration.Integration, map[string]string, error) {
	if r.env == nil {
		return integration.Integration{}, nil, errors.New("integrations: envelope not configured")
	}
	var i integration.Integration
	var err error
	if orgID == "" {
		i, err = r.getByID(ctx, id)
	} else {
		i, err = r.Get(ctx, orgID, id)
	}
	if err != nil {
		return integration.Integration{}, nil, err
	}
	idBytes, _ := ulidToBytes(string(id))
	var ct []byte
	err = r.db.QueryRowContext(ctx,
		`SELECT ciphertext FROM integration_credentials WHERE integration_id = ? LIMIT 1`,
		idBytes,
	).Scan(&ct)
	if errors.Is(err, sql.ErrNoRows) {
		return i, map[string]string{}, nil
	}
	if err != nil {
		return integration.Integration{}, nil, fmt.Errorf("integrations credentials select: %w", err)
	}
	pt, err := r.env.Decrypt(ct)
	if err != nil {
		return integration.Integration{}, nil, fmt.Errorf("integrations credentials decrypt: %w", err)
	}
	var secrets map[string]string
	if err := json.Unmarshal(pt, &secrets); err != nil {
		return integration.Integration{}, nil, fmt.Errorf("integrations credentials json: %w", err)
	}
	return i, secrets, nil
}

// getByID looks up an Integration by primary key alone. Used only by
// GetWithSecrets when the caller (webhook ingress) has no org context.
func (r *Integrations) getByID(ctx context.Context, id integration.ID) (integration.Integration, error) {
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return integration.Integration{}, fmt.Errorf("integrations id: %w", err)
	}
	const q = `SELECT id, org_id, type, provider, name, status, config, credentials_ref, capabilities, health, created_at, updated_at
	           FROM integrations WHERE id = ? LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, idBytes)
	return scanIntegration(row.Scan)
}

// scanIntegration decodes a row into integration.Integration.
func scanIntegration(scan func(dest ...any) error) (integration.Integration, error) {
	var (
		id, org, credRef []byte
		itype, provider  string
		name, status     string
		cfg, caps, hlth  []byte
		created, updated time.Time
	)
	if err := scan(&id, &org, &itype, &provider, &name, &status, &cfg, &credRef, &caps, &hlth, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return integration.Integration{}, ErrNotFound
		}
		return integration.Integration{}, fmt.Errorf("integrations scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return integration.Integration{}, fmt.Errorf("integrations bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return integration.Integration{}, fmt.Errorf("integrations bad org: %w", err)
	}
	out := integration.Integration{
		ID:        integration.ID(idStr),
		OrgID:     organization.ID(orgStr),
		Type:      integration.Type(itype),
		Provider:  provider,
		Name:      name,
		Status:    integrationStatusFromDB(status),
		CreatedAt: created,
		UpdatedAt: updated,
	}
	if len(credRef) > 0 {
		out.CredentialsRef = append([]byte(nil), credRef...)
	}
	if len(cfg) > 0 {
		var m map[string]any
		if err := json.Unmarshal(cfg, &m); err != nil {
			return integration.Integration{}, fmt.Errorf("integrations config json: %w", err)
		}
		out.Config = m
	}
	if len(caps) > 0 {
		var m map[string]bool
		if err := json.Unmarshal(caps, &m); err != nil {
			return integration.Integration{}, fmt.Errorf("integrations capabilities json: %w", err)
		}
		out.Capabilities = m
	}
	if len(hlth) > 0 {
		var m map[string]any
		if err := json.Unmarshal(hlth, &m); err != nil {
			return integration.Integration{}, fmt.Errorf("integrations health json: %w", err)
		}
		out.Health = m
	}
	return out, nil
}

// orEmptyMap returns m or an empty map so JSON columns never get 'null'.
func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// orEmptyBoolMap is the bool-valued sibling of orEmptyMap.
func orEmptyBoolMap(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return m
}
