package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	dusr "github.com/v-senthil/nudgeway/internal/domain/user"
)

// RBAC resolves the permission set for a (org, user) pair by joining
// user_roles → role_permissions. It exposes both the middleware's
// string-typed Resolve and a typed variant for the application layer via
// TypedResolver.
type RBAC struct {
	db *sql.DB
}

// NewRBAC constructs an RBAC resolver.
func NewRBAC(db *sql.DB) *RBAC { return &RBAC{db: db} }

// Resolve implements infrastructure/http/middleware.PermissionResolver.
// The middleware carries IDs as opaque strings on the request context.
func (r *RBAC) Resolve(ctx context.Context, orgID, userID string) (rbac.PermissionSet, error) {
	uid, err := ulidToBytes(userID)
	if err != nil {
		return nil, fmt.Errorf("rbac user_id: %w", err)
	}
	oid, err := ulidToBytes(orgID)
	if err != nil {
		return nil, fmt.Errorf("rbac org_id: %w", err)
	}
	const q = `
	  SELECT DISTINCT rp.permission_key
	  FROM user_roles ur
	  JOIN roles r        ON r.id = ur.role_id
	  JOIN role_permissions rp ON rp.role_id = r.id
	  WHERE ur.user_id = ? AND r.org_id = ?`
	rows, err := r.db.QueryContext(ctx, q, uid, oid)
	if err != nil {
		return nil, fmt.Errorf("rbac select: %w", err)
	}
	defer func() { _ = rows.Close() }()
	set := rbac.PermissionSet{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("rbac scan: %w", err)
		}
		set[rbac.Permission(k)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac rows: %w", err)
	}
	return set, nil
}

// TypedResolver adapts *RBAC to application/auth.PermissionResolver, whose
// Resolve method uses domain-typed identifiers rather than raw strings.
type TypedResolver struct{ r *RBAC }

// AsTyped returns an adapter satisfying application/auth.PermissionResolver.
func (r *RBAC) AsTyped() TypedResolver { return TypedResolver{r: r} }

// Resolve implements application/auth.PermissionResolver.
func (t TypedResolver) Resolve(ctx context.Context, orgID organization.ID, userID dusr.ID) (rbac.PermissionSet, error) {
	return t.r.Resolve(ctx, string(orgID), string(userID))
}
