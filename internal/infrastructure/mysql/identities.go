package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/identity"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// Identities implements repository.IdentityRepo against contact_identities.
type Identities struct {
	db *sql.DB
}

// NewIdentities constructs an Identities repository.
func NewIdentities(db *sql.DB) *Identities { return &Identities{db: db} }

// FindOrCreate looks up an identity by (org, provider, normalizedValue).
// If missing it inserts one bound to contactID. The upsert relies on
// INSERT ... ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id) so the
// subsequent read observes the winning row's ID under concurrency.
func (r *Identities) FindOrCreate(
	ctx context.Context,
	orgID organization.ID,
	contactID contact.ID,
	identityType identity.Type,
	provider string,
	value string,
	normalizedValue string,
) (identity.Identity, bool, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return identity.Identity{}, false, fmt.Errorf("identities org: %w", err)
	}
	ctBytes, err := ulidToBytes(string(contactID))
	if err != nil {
		return identity.Identity{}, false, fmt.Errorf("identities contact: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return identity.Identity{}, false, fmt.Errorf("identities tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	newID := newULID()
	const insertQ = `INSERT INTO contact_identities
	    (id, org_id, contact_id, identity_type, provider, identity_value, normalized_value, verified, metadata)
	  VALUES (?, ?, ?, ?, ?, ?, ?, 0, JSON_OBJECT())
	  ON DUPLICATE KEY UPDATE id = id`
	res, err := tx.ExecContext(ctx, insertQ,
		newID[:], orgBytes, ctBytes, string(identityType), provider, value, normalizedValue,
	)
	if err != nil {
		return identity.Identity{}, false, fmt.Errorf("identities insert: %w", err)
	}
	rows, _ := res.RowsAffected()
	created := rows == 1

	const selectQ = `SELECT id, org_id, contact_id, identity_type, provider, identity_value, normalized_value, verified, metadata, created_at, updated_at
	                 FROM contact_identities
	                 WHERE org_id = ? AND provider = ? AND normalized_value = ? LIMIT 1`
	row := tx.QueryRowContext(ctx, selectQ, orgBytes, provider, normalizedValue)
	got, err := scanIdentity(row.Scan)
	if err != nil {
		return identity.Identity{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return identity.Identity{}, false, fmt.Errorf("identities commit: %w", err)
	}
	return got, created, nil
}

// Get fetches an identity by (OrgID, ID).
func (r *Identities) Get(ctx context.Context, orgID organization.ID, id identity.ID) (identity.Identity, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identities org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identities id: %w", err)
	}
	const q = `SELECT id, org_id, contact_id, identity_type, provider, identity_value, normalized_value, verified, metadata, created_at, updated_at
	           FROM contact_identities WHERE org_id = ? AND id = ? LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, orgBytes, idBytes)
	return scanIdentity(row.Scan)
}

// ListForContact returns all identities bound to a Contact.
func (r *Identities) ListForContact(ctx context.Context, orgID organization.ID, contactID contact.ID) ([]identity.Identity, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("identities org: %w", err)
	}
	ctBytes, err := ulidToBytes(string(contactID))
	if err != nil {
		return nil, fmt.Errorf("identities contact: %w", err)
	}
	const q = `SELECT id, org_id, contact_id, identity_type, provider, identity_value, normalized_value, verified, metadata, created_at, updated_at
	           FROM contact_identities WHERE org_id = ? AND contact_id = ? ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, q, orgBytes, ctBytes)
	if err != nil {
		return nil, fmt.Errorf("identities select: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []identity.Identity
	for rows.Next() {
		got, err := scanIdentity(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identities rows: %w", err)
	}
	return out, nil
}

// scanIdentity decodes a row (from either QueryRow or Rows) into
// identity.Identity. It takes a Scan function so both callers work with
// one implementation.
func scanIdentity(scan func(dest ...any) error) (identity.Identity, error) {
	var (
		id, org, ct    []byte
		itype          string
		provider       string
		value          string
		norm           string
		verified       int
		metaBytes      []byte
		created, updat time.Time
	)
	if err := scan(&id, &org, &ct, &itype, &provider, &value, &norm, &verified, &metaBytes, &created, &updat); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Identity{}, ErrNotFound
		}
		return identity.Identity{}, fmt.Errorf("identities scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identities bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identities bad org: %w", err)
	}
	ctStr, err := ulidFromBytes(ct)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identities bad contact: %w", err)
	}
	var meta map[string]any
	if len(metaBytes) > 0 {
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			return identity.Identity{}, fmt.Errorf("identities metadata: %w", err)
		}
	}
	return identity.Identity{
		ID:              identity.ID(idStr),
		OrgID:           organization.ID(orgStr),
		ContactID:       contact.ID(ctStr),
		Type:            identity.Type(itype),
		Provider:        provider,
		Value:           value,
		NormalizedValue: norm,
		Verified:        verified != 0,
		Metadata:        meta,
		CreatedAt:       created,
		UpdatedAt:       updat,
	}, nil
}
