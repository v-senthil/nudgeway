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
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ErrNotFound is returned by every repository in this package when a
// requested row does not exist. Callers use errors.Is to distinguish
// missing rows from real errors.
var ErrNotFound = errors.New("mysql: row not found")

// Contacts implements repository.ContactRepo against the contacts table.
type Contacts struct {
	db *sql.DB
}

// NewContacts constructs a Contacts repository.
func NewContacts(db *sql.DB) *Contacts { return &Contacts{db: db} }

// Upsert inserts or updates a Contact identified by (OrgID, ID).
func (c *Contacts) Upsert(ctx context.Context, ct contact.Contact) error {
	idBytes, err := ulidToBytes(string(ct.ID))
	if err != nil {
		return fmt.Errorf("contacts id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(ct.OrgID))
	if err != nil {
		return fmt.Errorf("contacts org: %w", err)
	}
	var pidBytes any
	if ct.PrimaryIdentityID != nil {
		b, err := ulidToBytes(string(*ct.PrimaryIdentityID))
		if err != nil {
			return fmt.Errorf("contacts primary identity: %w", err)
		}
		pidBytes = b
	}
	var lastSeen any
	if ct.LastSeenAt != nil {
		lastSeen = ct.LastSeenAt.UTC()
	}
	var avatar any
	if ct.AvatarURL != "" {
		avatar = ct.AvatarURL
	}
	const q = `INSERT INTO contacts
	    (id, org_id, display_name, avatar_url, primary_identity_id, last_seen_at)
	  VALUES (?, ?, ?, ?, ?, ?)
	  ON DUPLICATE KEY UPDATE
	    display_name        = VALUES(display_name),
	    avatar_url          = VALUES(avatar_url),
	    primary_identity_id = VALUES(primary_identity_id),
	    last_seen_at        = VALUES(last_seen_at)`
	if _, err := c.db.ExecContext(ctx, q, idBytes, orgBytes, ct.DisplayName, avatar, pidBytes, lastSeen); err != nil {
		return fmt.Errorf("contacts upsert: %w", err)
	}
	return nil
}

// Get fetches a Contact by (OrgID, ID).
func (c *Contacts) Get(ctx context.Context, orgID organization.ID, id contact.ID) (contact.Contact, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return contact.Contact{}, fmt.Errorf("contacts org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return contact.Contact{}, fmt.Errorf("contacts id: %w", err)
	}
	const q = `SELECT id, org_id, display_name, avatar_url, primary_identity_id, last_seen_at, created_at, updated_at
	           FROM contacts WHERE org_id = ? AND id = ? LIMIT 1`
	return c.scanRow(c.db.QueryRowContext(ctx, q, orgBytes, idBytes))
}

// FindByPrimaryIdentity returns the contact whose PrimaryIdentityID
// points at the given identity, if any.
func (c *Contacts) FindByPrimaryIdentity(ctx context.Context, orgID organization.ID, identityID contact.IdentityID) (contact.Contact, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return contact.Contact{}, fmt.Errorf("contacts org: %w", err)
	}
	pidBytes, err := ulidToBytes(string(identityID))
	if err != nil {
		return contact.Contact{}, fmt.Errorf("contacts identity id: %w", err)
	}
	const q = `SELECT id, org_id, display_name, avatar_url, primary_identity_id, last_seen_at, created_at, updated_at
	           FROM contacts WHERE org_id = ? AND primary_identity_id = ? LIMIT 1`
	return c.scanRow(c.db.QueryRowContext(ctx, q, orgBytes, pidBytes))
}

// List returns contacts for the org, filtered + paginated.
func (c *Contacts) List(ctx context.Context, orgID organization.ID, filter repository.ContactListFilter) (repository.ContactPage, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.ContactPage{}, fmt.Errorf("contacts org: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var (
		conds = []string{"org_id = ?"}
		args  = []any{orgBytes}
	)
	if filter.Cursor != "" {
		cur, err := ulidToBytes(filter.Cursor)
		if err != nil {
			return repository.ContactPage{}, fmt.Errorf("contacts cursor: %w", err)
		}
		conds = append(conds, "id > ?")
		args = append(args, cur)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		conds = append(conds, "display_name LIKE ?")
		args = append(args, "%"+q+"%")
	}
	sqlStr := "SELECT id, org_id, display_name, avatar_url, primary_identity_id, last_seen_at, created_at, updated_at " +
		"FROM contacts WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := c.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return repository.ContactPage{}, fmt.Errorf("contacts list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := repository.ContactPage{}
	for rows.Next() {
		ct, err := c.scanNext(rows)
		if err != nil {
			return repository.ContactPage{}, err
		}
		out.Contacts = append(out.Contacts, ct)
	}
	if err := rows.Err(); err != nil {
		return repository.ContactPage{}, fmt.Errorf("contacts list rows: %w", err)
	}
	if len(out.Contacts) > limit {
		last := out.Contacts[limit-1]
		out.Contacts = out.Contacts[:limit]
		out.NextCursor = string(last.ID)
	}
	return out, nil
}

// scanRow scans a single *sql.Row into a Contact.
func (c *Contacts) scanRow(row *sql.Row) (contact.Contact, error) {
	var (
		id, org, pid []byte
		display      string
		avatar       sql.NullString
		lastSeen     sql.NullTime
		created, upd time.Time
	)
	if err := row.Scan(&id, &org, &display, &avatar, &pid, &lastSeen, &created, &upd); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contact.Contact{}, ErrNotFound
		}
		return contact.Contact{}, fmt.Errorf("contacts scan: %w", err)
	}
	return hydrateContact(id, org, pid, display, avatar, lastSeen, created, upd)
}

// scanNext scans the current *sql.Rows position into a Contact.
func (c *Contacts) scanNext(rows *sql.Rows) (contact.Contact, error) {
	var (
		id, org, pid []byte
		display      string
		avatar       sql.NullString
		lastSeen     sql.NullTime
		created, upd time.Time
	)
	if err := rows.Scan(&id, &org, &display, &avatar, &pid, &lastSeen, &created, &upd); err != nil {
		return contact.Contact{}, fmt.Errorf("contacts scan: %w", err)
	}
	return hydrateContact(id, org, pid, display, avatar, lastSeen, created, upd)
}

// hydrateContact assembles a contact.Contact from raw column values.
func hydrateContact(id, org, pid []byte, display string, avatar sql.NullString, lastSeen sql.NullTime, created, updated time.Time) (contact.Contact, error) {
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return contact.Contact{}, fmt.Errorf("contacts bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return contact.Contact{}, fmt.Errorf("contacts bad org: %w", err)
	}
	out := contact.Contact{
		ID:          contact.ID(idStr),
		OrgID:       organization.ID(orgStr),
		DisplayName: display,
		CreatedAt:   created,
		UpdatedAt:   updated,
	}
	if avatar.Valid {
		out.AvatarURL = avatar.String
	}
	if len(pid) == 16 {
		pidStr, err := ulidFromBytes(pid)
		if err != nil {
			return contact.Contact{}, fmt.Errorf("contacts bad primary identity: %w", err)
		}
		iid := contact.IdentityID(pidStr)
		out.PrimaryIdentityID = &iid
	}
	if lastSeen.Valid {
		t := lastSeen.Time
		out.LastSeenAt = &t
	}
	return out, nil
}

// jsonMustMarshal marshals v to JSON; on any error returns "null" — the
// call sites here only ever marshal maps/slices we ourselves populated,
// so failure is a programming bug we surface loudly in logs but never
// crash on.
func jsonMustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}
