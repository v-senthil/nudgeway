package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/fullwa/fullwa/internal/domain/organization"
	dusr "github.com/fullwa/fullwa/internal/domain/user"
)

// Users implements application/auth.UserFinder against the users table.
type Users struct {
	db *sql.DB
}

// NewUsers constructs a Users repository.
func NewUsers(db *sql.DB) *Users { return &Users{db: db} }

// ErrUserNotFound is returned when no user matches the lookup.
var ErrUserNotFound = errors.New("user not found")

// FindByEmail returns the active user with the given normalized email.
// Emails are unique per (org, email); we look up across all orgs because
// authentication is org-agnostic at the URL layer. Multi-org login by
// email disambiguation is a Phase 1 feature; for Phase 0 we return the
// first match.
func (u *Users) FindByEmail(ctx context.Context, email string) (dusr.User, error) {
	const q = `SELECT id, org_id, email, password_hash, display_name, status, last_login_at, created_at, updated_at
	           FROM users WHERE email = ? LIMIT 1`
	row := u.db.QueryRowContext(ctx, q, email)
	var (
		idBytes    []byte
		orgIDBytes []byte
		mail       string
		pwHash     []byte
		display    string
		status     string
		lastLogin  sql.NullTime
		createdAt  time.Time
		updatedAt  time.Time
	)
	if err := row.Scan(&idBytes, &orgIDBytes, &mail, &pwHash, &display, &status, &lastLogin, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dusr.User{}, ErrUserNotFound
		}
		return dusr.User{}, fmt.Errorf("users select: %w", err)
	}
	uid, err := ulidFromBytes(idBytes)
	if err != nil {
		return dusr.User{}, fmt.Errorf("users bad id: %w", err)
	}
	oid, err := ulidFromBytes(orgIDBytes)
	if err != nil {
		return dusr.User{}, fmt.Errorf("users bad org_id: %w", err)
	}
	out := dusr.User{
		ID:           dusr.ID(uid),
		OrgID:        organization.ID(oid),
		Email:        mail,
		PasswordHash: pwHash,
		DisplayName:  display,
		Status:       dusr.Status(status),
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		out.LastLoginAt = &t
	}
	return out, nil
}

// ulidFromBytes decodes a 16-byte ULID/UUID into its canonical string form.
func ulidFromBytes(b []byte) (string, error) {
	if len(b) != 16 {
		return "", fmt.Errorf("expected 16 bytes, got %d", len(b))
	}
	var id ulid.ULID
	copy(id[:], b)
	return id.String(), nil
}

// ulidToBytes decodes the canonical string form to 16 raw bytes.
func ulidToBytes(s string) ([]byte, error) {
	id, err := ulid.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse ulid %q: %w", s, err)
	}
	b := make([]byte, 16)
	copy(b, id[:])
	return b, nil
}
