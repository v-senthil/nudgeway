package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	dgroup "github.com/v-senthil/nudgeway/internal/domain/group"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// Groups implements repository.GroupRepo against the `groups` +
// `group_members` tables.
type Groups struct {
	db *sql.DB
}

// NewGroups constructs a Groups repository.
func NewGroups(db *sql.DB) *Groups { return &Groups{db: db} }

// Upsert inserts or updates a Group identified by
// (OrgID, IntegrationID, ProviderGroupID). Returns the persisted row.
func (r *Groups) Upsert(ctx context.Context, g dgroup.Group) (dgroup.Group, error) {
	orgBytes, err := ulidToBytes(string(g.OrgID))
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups org: %w", err)
	}
	integBytes, err := ulidToBytes(string(g.IntegrationID))
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups integration: %w", err)
	}
	// Resolve the target row id via the natural key so we can return a
	// stable domain ID on every call (not the DB-side rowid).
	existing, err := r.GetByProviderID(ctx, g.OrgID, g.IntegrationID, g.ProviderGroupID)
	var id []byte
	switch {
	case err == nil:
		id, err = ulidToBytes(string(existing.ID))
		if err != nil {
			return dgroup.Group{}, fmt.Errorf("groups existing id: %w", err)
		}
	case errors.Is(err, dgroup.ErrNotFound):
		id = newULID().Bytes()
	default:
		return dgroup.Group{}, fmt.Errorf("groups lookup: %w", err)
	}

	md := g.Metadata
	if md == nil {
		md = map[string]any{}
	}
	mdJSON, err := json.Marshal(md)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups metadata: %w", err)
	}
	var desc any
	if g.Description != "" {
		desc = g.Description
	}
	const q = "INSERT INTO `groups`" + `
	    (id, org_id, integration_id, provider_group_id, subject, description, size, is_admin, metadata)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	  ON DUPLICATE KEY UPDATE
	    subject     = VALUES(subject),
	    description = VALUES(description),
	    size        = VALUES(size),
	    is_admin    = VALUES(is_admin),
	    metadata    = VALUES(metadata)`
	if _, err := r.db.ExecContext(ctx, q,
		id, orgBytes, integBytes, g.ProviderGroupID,
		g.Subject, desc, g.Size, g.IsAdmin, mdJSON,
	); err != nil {
		return dgroup.Group{}, fmt.Errorf("groups upsert: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups bad id: %w", err)
	}
	return r.Get(ctx, g.OrgID, dgroup.ID(idStr))
}

// Get fetches a Group by (OrgID, ID).
func (r *Groups) Get(ctx context.Context, orgID organization.ID, id dgroup.ID) (dgroup.Group, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups id: %w", err)
	}
	const q = "SELECT id, org_id, integration_id, provider_group_id, subject, description, size, is_admin, metadata, created_at, updated_at " +
		"FROM `groups` WHERE org_id = ? AND id = ? LIMIT 1"
	return r.scanRow(r.db.QueryRowContext(ctx, q, orgBytes, idBytes))
}

// GetByProviderID resolves a Group by its provider-native id under a
// specific integration.
func (r *Groups) GetByProviderID(ctx context.Context, orgID organization.ID, integrationID integration.ID, providerGroupID string) (dgroup.Group, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups org: %w", err)
	}
	integBytes, err := ulidToBytes(string(integrationID))
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups integration: %w", err)
	}
	const q = "SELECT id, org_id, integration_id, provider_group_id, subject, description, size, is_admin, metadata, created_at, updated_at " +
		"FROM `groups` WHERE org_id = ? AND integration_id = ? AND provider_group_id = ? LIMIT 1"
	return r.scanRow(r.db.QueryRowContext(ctx, q, orgBytes, integBytes, providerGroupID))
}

// List returns groups for the org, filtered + paginated by ULID cursor.
func (r *Groups) List(ctx context.Context, orgID organization.ID, filter repository.GroupListFilter) ([]dgroup.Group, string, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, "", fmt.Errorf("groups org: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	conds := []string{"org_id = ?"}
	args := []any{orgBytes}
	if string(filter.IntegrationID) != "" {
		integBytes, err := ulidToBytes(string(filter.IntegrationID))
		if err != nil {
			return nil, "", fmt.Errorf("groups integration: %w", err)
		}
		conds = append(conds, "integration_id = ?")
		args = append(args, integBytes)
	}
	if filter.Cursor != "" {
		cur, err := ulidToBytes(filter.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("groups cursor: %w", err)
		}
		conds = append(conds, "id > ?")
		args = append(args, cur)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		conds = append(conds, "subject LIKE ?")
		args = append(args, "%"+q+"%")
	}
	sqlStr := "SELECT id, org_id, integration_id, provider_group_id, subject, description, size, is_admin, metadata, created_at, updated_at " +
		"FROM `groups` WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", fmt.Errorf("groups list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []dgroup.Group{}
	for rows.Next() {
		g, err := r.scanNext(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("groups list rows: %w", err)
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		out = out[:limit]
		next = string(last.ID)
	}
	return out, next, nil
}

// Delete removes a Group and cascades to its members.
func (r *Groups) Delete(ctx context.Context, orgID organization.ID, id dgroup.ID) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("groups org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("groups id: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM group_members WHERE org_id = ? AND group_id = ?`,
		orgBytes, idBytes,
	); err != nil {
		return fmt.Errorf("groups members delete: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		"DELETE FROM `groups` WHERE org_id = ? AND id = ?",
		orgBytes, idBytes,
	); err != nil {
		return fmt.Errorf("groups delete: %w", err)
	}
	return nil
}

// AddMember upserts a member row keyed by (GroupID, WaID, BSUID).
func (r *Groups) AddMember(ctx context.Context, m dgroup.Member) error {
	orgBytes, err := ulidToBytes(string(m.OrgID))
	if err != nil {
		return fmt.Errorf("group members org: %w", err)
	}
	groupBytes, err := ulidToBytes(string(m.GroupID))
	if err != nil {
		return fmt.Errorf("group members group: %w", err)
	}
	var contactBytes any
	if m.ContactID != nil && *m.ContactID != "" {
		cb, err := ulidToBytes(string(*m.ContactID))
		if err != nil {
			return fmt.Errorf("group members contact: %w", err)
		}
		contactBytes = cb
	}
	role := string(m.Role)
	if role == "" {
		role = string(dgroup.RoleMember)
	}
	const q = `INSERT INTO group_members
	    (org_id, group_id, contact_id, wa_id, bsuid, role, joined_at, left_at)
	  VALUES (?, ?, ?, ?, ?, ?, ?, NULL)
	  ON DUPLICATE KEY UPDATE
	    contact_id = VALUES(contact_id),
	    role       = VALUES(role),
	    left_at    = NULL`
	joinedAt := m.JoinedAt
	if joinedAt.IsZero() {
		joinedAt = time.Now().UTC()
	}
	if _, err := r.db.ExecContext(ctx, q,
		orgBytes, groupBytes, contactBytes, m.WaID, m.BSUID, role, joinedAt,
	); err != nil {
		return fmt.Errorf("group members upsert: %w", err)
	}
	return nil
}

// RemoveMember stamps LeftAt on the identified row. Idempotent.
func (r *Groups) RemoveMember(ctx context.Context, orgID organization.ID, groupID dgroup.ID, key repository.GroupMemberKey) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("group members org: %w", err)
	}
	groupBytes, err := ulidToBytes(string(groupID))
	if err != nil {
		return fmt.Errorf("group members group: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE group_members SET left_at = ?
		 WHERE org_id = ? AND group_id = ? AND wa_id = ? AND bsuid = ? AND left_at IS NULL`,
		time.Now().UTC(), orgBytes, groupBytes, key.WaID, key.BSUID,
	); err != nil {
		return fmt.Errorf("group members remove: %w", err)
	}
	return nil
}

// ListMembers returns the active + historical member roster for the group.
func (r *Groups) ListMembers(ctx context.Context, orgID organization.ID, groupID dgroup.ID) ([]dgroup.Member, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("group members org: %w", err)
	}
	groupBytes, err := ulidToBytes(string(groupID))
	if err != nil {
		return nil, fmt.Errorf("group members group: %w", err)
	}
	const q = `SELECT id, org_id, group_id, contact_id, wa_id, bsuid, role, joined_at, left_at
	           FROM group_members WHERE org_id = ? AND group_id = ?
	           ORDER BY joined_at ASC`
	rows, err := r.db.QueryContext(ctx, q, orgBytes, groupBytes)
	if err != nil {
		return nil, fmt.Errorf("group members list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []dgroup.Member{}
	for rows.Next() {
		var (
			id              uint64
			orgB, gB, cB    []byte
			waID, bsuid, rl string
			joined          time.Time
			left            sql.NullTime
		)
		if err := rows.Scan(&id, &orgB, &gB, &cB, &waID, &bsuid, &rl, &joined, &left); err != nil {
			return nil, fmt.Errorf("group members scan: %w", err)
		}
		m, err := hydrateMember(id, orgB, gB, cB, waID, bsuid, rl, joined, left)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("group members rows: %w", err)
	}
	return out, nil
}

// scanRow scans a single *sql.Row into a Group.
func (r *Groups) scanRow(row *sql.Row) (dgroup.Group, error) {
	var (
		id, org, integ []byte
		providerID     string
		subject        string
		desc           sql.NullString
		size           int
		isAdmin        bool
		mdRaw          []byte
		created, upd   time.Time
	)
	if err := row.Scan(&id, &org, &integ, &providerID, &subject, &desc, &size, &isAdmin, &mdRaw, &created, &upd); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dgroup.Group{}, dgroup.ErrNotFound
		}
		return dgroup.Group{}, fmt.Errorf("groups scan: %w", err)
	}
	return hydrateGroup(id, org, integ, providerID, subject, desc, size, isAdmin, mdRaw, created, upd)
}

// scanNext scans the current *sql.Rows position into a Group.
func (r *Groups) scanNext(rows *sql.Rows) (dgroup.Group, error) {
	var (
		id, org, integ []byte
		providerID     string
		subject        string
		desc           sql.NullString
		size           int
		isAdmin        bool
		mdRaw          []byte
		created, upd   time.Time
	)
	if err := rows.Scan(&id, &org, &integ, &providerID, &subject, &desc, &size, &isAdmin, &mdRaw, &created, &upd); err != nil {
		return dgroup.Group{}, fmt.Errorf("groups scan: %w", err)
	}
	return hydrateGroup(id, org, integ, providerID, subject, desc, size, isAdmin, mdRaw, created, upd)
}

// hydrateGroup assembles a domain Group from raw column values.
func hydrateGroup(id, org, integ []byte, providerID, subject string, desc sql.NullString, size int, isAdmin bool, mdRaw []byte, created, updated time.Time) (dgroup.Group, error) {
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups bad org: %w", err)
	}
	integStr, err := ulidFromBytes(integ)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("groups bad integration: %w", err)
	}
	md := map[string]any{}
	if len(mdRaw) > 0 {
		_ = json.Unmarshal(mdRaw, &md)
	}
	out := dgroup.Group{
		ID:              dgroup.ID(idStr),
		OrgID:           organization.ID(orgStr),
		IntegrationID:   integration.ID(integStr),
		ProviderGroupID: providerID,
		Subject:         subject,
		Size:            size,
		IsAdmin:         isAdmin,
		Metadata:        md,
		CreatedAt:       created,
		UpdatedAt:       updated,
	}
	if desc.Valid {
		out.Description = desc.String
	}
	return out, nil
}

// hydrateMember assembles a Member from raw column values.
func hydrateMember(id uint64, org, groupID, contactB []byte, waID, bsuid, role string, joined time.Time, left sql.NullTime) (dgroup.Member, error) {
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return dgroup.Member{}, fmt.Errorf("group members bad org: %w", err)
	}
	gStr, err := ulidFromBytes(groupID)
	if err != nil {
		return dgroup.Member{}, fmt.Errorf("group members bad group: %w", err)
	}
	m := dgroup.Member{
		ID:       id,
		OrgID:    organization.ID(orgStr),
		GroupID:  dgroup.ID(gStr),
		WaID:     waID,
		BSUID:    bsuid,
		Role:     dgroup.Role(role),
		JoinedAt: joined,
	}
	if len(contactB) == 16 {
		cStr, err := ulidFromBytes(contactB)
		if err != nil {
			return dgroup.Member{}, fmt.Errorf("group members bad contact: %w", err)
		}
		cid := contact.ID(cStr)
		m.ContactID = &cid
	}
	if left.Valid {
		t := left.Time
		m.LeftAt = &t
	}
	return m, nil
}
