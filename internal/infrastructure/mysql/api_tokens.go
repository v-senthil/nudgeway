package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/user"
)

// APITokens implements repository.APITokenRepo against the api_tokens table.
type APITokens struct {
	db *sql.DB
}

// NewAPITokens constructs an APITokens repository.
func NewAPITokens(db *sql.DB) *APITokens { return &APITokens{db: db} }

// Create inserts a new api_tokens row.
func (r *APITokens) Create(ctx context.Context, t apitoken.Token) error {
	idBytes, err := ulidToBytes(string(t.ID))
	if err != nil {
		return fmt.Errorf("api_tokens id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(t.OrgID))
	if err != nil {
		return fmt.Errorf("api_tokens org: %w", err)
	}
	userBytes, err := ulidToBytes(string(t.UserID))
	if err != nil {
		return fmt.Errorf("api_tokens user: %w", err)
	}
	var expires any
	if t.ExpiresAt != nil {
		expires = t.ExpiresAt.UTC()
	}
	created := t.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	const q = `INSERT INTO api_tokens
	    (id, org_id, user_id, name, prefix, secret_hash, expires_at, created_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, q,
		idBytes, orgBytes, userBytes, t.Name, t.Prefix, t.SecretHash, expires, created.UTC(),
	); err != nil {
		return fmt.Errorf("api_tokens insert: %w", err)
	}
	return nil
}

// ListByOrg returns every token owned by the org, newest-first.
func (r *APITokens) ListByOrg(ctx context.Context, orgID organization.ID) ([]apitoken.Token, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("api_tokens org: %w", err)
	}
	const q = `SELECT id, org_id, user_id, name, prefix, secret_hash, last_used_at, expires_at, created_at, revoked_at
	           FROM api_tokens WHERE org_id = ? ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, q, orgBytes)
	if err != nil {
		return nil, fmt.Errorf("api_tokens list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []apitoken.Token
	for rows.Next() {
		t, err := scanAPIToken(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("api_tokens rows: %w", err)
	}
	return out, nil
}

// LookupByPrefix returns the token whose plaintext prefix matches.
func (r *APITokens) LookupByPrefix(ctx context.Context, prefix string) (apitoken.Token, error) {
	const q = `SELECT id, org_id, user_id, name, prefix, secret_hash, last_used_at, expires_at, created_at, revoked_at
	           FROM api_tokens WHERE prefix = ? LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, prefix)
	t, err := scanAPIToken(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apitoken.Token{}, apitoken.ErrNotFound
		}
		return apitoken.Token{}, err
	}
	return t, nil
}

// TouchLastUsed stamps last_used_at for a token id.
func (r *APITokens) TouchLastUsed(ctx context.Context, id apitoken.ID, when time.Time) error {
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("api_tokens id: %w", err)
	}
	const q = `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, q, when.UTC(), idBytes); err != nil {
		return fmt.Errorf("api_tokens touch: %w", err)
	}
	return nil
}

// Revoke stamps revoked_at for a token owned by orgID.
func (r *APITokens) Revoke(ctx context.Context, orgID organization.ID, id apitoken.ID, when time.Time) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("api_tokens org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("api_tokens id: %w", err)
	}
	const q = `UPDATE api_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE org_id = ? AND id = ?`
	res, err := r.db.ExecContext(ctx, q, when.UTC(), orgBytes, idBytes)
	if err != nil {
		return fmt.Errorf("api_tokens revoke: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("api_tokens revoke rows: %w", err)
	}
	if n == 0 {
		return apitoken.ErrNotFound
	}
	return nil
}

// scanAPIToken decodes a single row into an apitoken.Token.
func scanAPIToken(scan func(...any) error) (apitoken.Token, error) {
	var (
		idBytes    []byte
		orgBytes   []byte
		userBytes  []byte
		name       string
		prefix     string
		secretHash []byte
		lastUsed   sql.NullTime
		expires    sql.NullTime
		createdAt  time.Time
		revokedAt  sql.NullTime
	)
	if err := scan(&idBytes, &orgBytes, &userBytes, &name, &prefix, &secretHash, &lastUsed, &expires, &createdAt, &revokedAt); err != nil {
		return apitoken.Token{}, err
	}
	idStr, err := ulidFromBytes(idBytes)
	if err != nil {
		return apitoken.Token{}, fmt.Errorf("api_tokens bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(orgBytes)
	if err != nil {
		return apitoken.Token{}, fmt.Errorf("api_tokens bad org: %w", err)
	}
	userStr, err := ulidFromBytes(userBytes)
	if err != nil {
		return apitoken.Token{}, fmt.Errorf("api_tokens bad user: %w", err)
	}
	out := apitoken.Token{
		ID:         apitoken.ID(idStr),
		OrgID:      organization.ID(orgStr),
		UserID:     user.ID(userStr),
		Name:       name,
		Prefix:     prefix,
		SecretHash: secretHash,
		CreatedAt:  createdAt,
	}
	if lastUsed.Valid {
		t := lastUsed.Time
		out.LastUsedAt = &t
	}
	if expires.Valid {
		t := expires.Time
		out.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		out.RevokedAt = &t
	}
	return out, nil
}
