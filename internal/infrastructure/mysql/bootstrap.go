package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/fullwa/fullwa/internal/domain/rbac"
	dusr "github.com/fullwa/fullwa/internal/domain/user"
	infauth "github.com/fullwa/fullwa/internal/infrastructure/auth"
)

// Bootstrap wraps the CLI's admin-provisioning operations.
type Bootstrap struct {
	db  *sql.DB
	arg infauth.Argon2Params
}

// NewBootstrap constructs a Bootstrap helper.
func NewBootstrap(db *sql.DB, arg infauth.Argon2Params) *Bootstrap {
	return &Bootstrap{db: db, arg: arg}
}

// CreateOrg inserts an organization row. Returns the canonical ULID string.
func (b *Bootstrap) CreateOrg(ctx context.Context, slug, name string) (string, error) {
	if slug == "" || name == "" {
		return "", errors.New("bootstrap: slug and name required")
	}
	id := newULID()
	const q = `INSERT INTO organizations (id, slug, name, status, settings) VALUES (?, ?, ?, 'active', JSON_OBJECT())`
	if _, err := b.db.ExecContext(ctx, q, id[:], slug, name); err != nil {
		return "", fmt.Errorf("insert organization: %w", err)
	}
	return id.String(), nil
}

// LookupOrgBySlug returns the ULID string of the org with the given slug.
func (b *Bootstrap) LookupOrgBySlug(ctx context.Context, slug string) (string, error) {
	const q = `SELECT id FROM organizations WHERE slug = ? LIMIT 1`
	var raw []byte
	if err := b.db.QueryRowContext(ctx, q, slug).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("org %q not found", slug)
		}
		return "", fmt.Errorf("select org: %w", err)
	}
	return ulidFromBytes(raw)
}

// CreateUser inserts a user (status=active) with an argon2id password hash
// and returns its canonical ULID string.
func (b *Bootstrap) CreateUser(ctx context.Context, orgID, email, password string) (string, error) {
	normEmail, err := dusr.NormalizeEmail(email)
	if err != nil {
		return "", fmt.Errorf("bootstrap email: %w", err)
	}
	oid, err := ulidToBytes(orgID)
	if err != nil {
		return "", err
	}
	pw, err := infauth.HashPassword(password, b.arg)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	id := newULID()
	const q = `INSERT INTO users (id, org_id, email, password_hash, display_name, status)
	           VALUES (?, ?, ?, ?, ?, 'active')`
	if _, err := b.db.ExecContext(ctx, q, id[:], oid, normEmail, []byte(pw), ""); err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	return id.String(), nil
}

// EnsureAdminRole creates (or reuses) the system "admin" role for orgID that
// grants every permission returned by rbac.All(). Returns the role ID.
func (b *Bootstrap) EnsureAdminRole(ctx context.Context, orgID string) (string, error) {
	oid, err := ulidToBytes(orgID)
	if err != nil {
		return "", err
	}
	// look up first.
	var raw []byte
	err = b.db.QueryRowContext(ctx, `SELECT id FROM roles WHERE org_id = ? AND name = 'admin' LIMIT 1`, oid).Scan(&raw)
	if err == nil {
		return ulidFromBytes(raw)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("select admin role: %w", err)
	}
	id := newULID()
	if _, err := b.db.ExecContext(ctx,
		`INSERT INTO roles (id, org_id, name, is_system) VALUES (?, ?, 'admin', 1)`,
		id[:], oid,
	); err != nil {
		return "", fmt.Errorf("insert admin role: %w", err)
	}
	for _, p := range rbac.All() {
		if _, err := b.db.ExecContext(ctx,
			`INSERT INTO role_permissions (role_id, permission_key) VALUES (?, ?)`,
			id[:], string(p),
		); err != nil {
			return "", fmt.Errorf("insert role_permission %s: %w", p, err)
		}
	}
	return id.String(), nil
}

// AssignRole binds a role to a user (idempotent).
func (b *Bootstrap) AssignRole(ctx context.Context, userID, roleID string) error {
	uid, err := ulidToBytes(userID)
	if err != nil {
		return err
	}
	rid, err := ulidToBytes(roleID)
	if err != nil {
		return err
	}
	_, err = b.db.ExecContext(ctx,
		`INSERT IGNORE INTO user_roles (user_id, role_id) VALUES (?, ?)`,
		uid, rid,
	)
	if err != nil {
		return fmt.Errorf("insert user_role: %w", err)
	}
	return nil
}

// newULID generates a fresh ULID using time.Now.
func newULID() ulid.ULID {
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulid.DefaultEntropy())
}

// Orgs is a small read-only repository for organizations, wired to the
// /api/v1/auth/me handler for org-name lookups.
type Orgs struct{ db *sql.DB }

// NewOrgs constructs an Orgs repository.
func NewOrgs(db *sql.DB) *Orgs { return &Orgs{db: db} }

// Name returns the display name of the org with the given canonical ID.
func (o *Orgs) Name(ctx context.Context, orgID string) (string, error) {
	oid, err := ulidToBytes(orgID)
	if err != nil {
		return "", err
	}
	var name string
	if err := o.db.QueryRowContext(ctx, `SELECT name FROM organizations WHERE id = ? LIMIT 1`, oid).Scan(&name); err != nil {
		return "", fmt.Errorf("select org name: %w", err)
	}
	return name, nil
}
