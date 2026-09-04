package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/fullwa/fullwa/internal/domain/audit"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/user"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// Audit implements repository.AuditRepo against MySQL. Rows land in the
// audit_logs table declared in migrations/20260903000001_organizations_users_roles.
type Audit struct {
	db *sql.DB
}

// NewAudit constructs an Audit repository against db.
func NewAudit(db *sql.DB) *Audit { return &Audit{db: db} }

// Record inserts an entry and returns its assigned primary key.
//
// OrgID and Action are required — a missing value returns
// audit.ErrInvalidEntry so wire-up bugs surface loudly. OccurredAt is
// stamped to time.Now().UTC() when the caller left it zero.
func (a *Audit) Record(ctx context.Context, e audit.Entry) (uint64, error) {
	if e.OrgID == "" || e.Action == "" {
		return 0, audit.ErrInvalidEntry
	}
	orgBytes, err := ulidToBytes(string(e.OrgID))
	if err != nil {
		return 0, fmt.Errorf("audit org: %w", err)
	}
	var actorBytes any
	if e.ActorUserID != nil {
		b, err := ulidToBytes(string(*e.ActorUserID))
		if err != nil {
			return 0, fmt.Errorf("audit actor: %w", err)
		}
		actorBytes = b
	}
	var ipBytes any
	if len(e.IP) > 0 {
		// Normalise to the 4- or 16-byte representation MySQL's INET6_NTOA
		// expects. VARBINARY(16) accepts either shape; picking canonical
		// form keeps admin queries simple.
		if v4 := e.IP.To4(); v4 != nil {
			ipBytes = []byte(v4)
		} else {
			ipBytes = []byte(e.IP.To16())
		}
	}
	occurred := e.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	} else {
		occurred = occurred.UTC()
	}
	meta := jsonMustMarshal(orEmptyMap(e.Metadata))

	const q = `INSERT INTO audit_logs
	    (org_id, actor_user_id, action, resource_type, resource_id, ip, metadata, occurred_at)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := a.db.ExecContext(ctx, q,
		orgBytes, actorBytes, string(e.Action), e.ResourceType, e.ResourceID,
		ipBytes, meta, occurred,
	)
	if err != nil {
		return 0, fmt.Errorf("audit insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("audit lastid: %w", err)
	}
	return uint64(id), nil
}

// List returns a page of entries newest-first for the given org.
//
// Pagination uses the (occurred_at, id) tuple encoded as opaque base64
// so the caller can neither read nor mutate the cursor. The composite
// (org_id, occurred_at) index drives the sort order.
func (a *Audit) List(
	ctx context.Context,
	orgID organization.ID,
	filter repository.AuditListFilter,
) ([]audit.Entry, string, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, "", fmt.Errorf("audit org: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	conds := []string{"org_id = ?"}
	args := []any{orgBytes}
	if filter.ResourceType != "" {
		conds = append(conds, "resource_type = ?")
		args = append(args, filter.ResourceType)
	}
	if filter.ResourceID != "" {
		conds = append(conds, "resource_id = ?")
		args = append(args, filter.ResourceID)
	}
	if filter.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, string(filter.Action))
	}
	if filter.ActorUserID != nil {
		b, err := ulidToBytes(string(*filter.ActorUserID))
		if err != nil {
			return nil, "", fmt.Errorf("audit actor filter: %w", err)
		}
		conds = append(conds, "actor_user_id = ?")
		args = append(args, b)
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "occurred_at >= ?")
		args = append(args, filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "occurred_at < ?")
		args = append(args, filter.Until.UTC())
	}
	if filter.Cursor != "" {
		cAt, cID, err := decodeAuditCursor(filter.Cursor)
		if err != nil {
			return nil, "", err
		}
		conds = append(conds, "(occurred_at, id) < (?, ?)")
		args = append(args, cAt.UTC(), cID)
	}

	q := "SELECT id, org_id, actor_user_id, action, resource_type, resource_id, ip, metadata, occurred_at " +
		"FROM audit_logs WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY occurred_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("audit list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]audit.Entry, 0, limit)
	for rows.Next() {
		e, err := scanAuditRow(rows.Scan)
		if err != nil {
			return nil, "", err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("audit rows: %w", err)
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		out = out[:limit]
		next = encodeAuditCursor(last.OccurredAt, last.ID)
	}
	return out, next, nil
}

// scanAuditRow decodes one audit_logs row into a domain Entry.
func scanAuditRow(scan func(dest ...any) error) (audit.Entry, error) {
	var (
		id        uint64
		orgB      []byte
		actorB    []byte
		action    string
		rtype     string
		rid       string
		ipB       []byte
		metaBytes []byte
		occurred  time.Time
	)
	if err := scan(&id, &orgB, &actorB, &action, &rtype, &rid, &ipB, &metaBytes, &occurred); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return audit.Entry{}, ErrNotFound
		}
		return audit.Entry{}, fmt.Errorf("audit scan: %w", err)
	}
	orgStr, err := ulidFromBytes(orgB)
	if err != nil {
		return audit.Entry{}, fmt.Errorf("audit bad org: %w", err)
	}
	e := audit.Entry{
		ID:           id,
		OrgID:        organization.ID(orgStr),
		Action:       audit.Action(action),
		ResourceType: rtype,
		ResourceID:   rid,
		OccurredAt:   occurred,
	}
	if len(actorB) == 16 {
		s, err := ulidFromBytes(actorB)
		if err != nil {
			return audit.Entry{}, fmt.Errorf("audit bad actor: %w", err)
		}
		uid := user.ID(s)
		e.ActorUserID = &uid
	}
	if len(ipB) > 0 {
		e.IP = net.IP(ipB)
	}
	if len(metaBytes) > 0 {
		var m map[string]any
		if err := json.Unmarshal(metaBytes, &m); err != nil {
			return audit.Entry{}, fmt.Errorf("audit metadata: %w", err)
		}
		e.Metadata = m
	}
	return e, nil
}

// encodeAuditCursor packs (occurred_at, id) into an opaque base64 token.
// The plain form is "<unix_micros>|<id>" — kept purposefully opaque so
// the frontend cannot forge or interpret it.
func encodeAuditCursor(at time.Time, id uint64) string {
	plain := strconv.FormatInt(at.UTC().UnixMicro(), 10) + "|" + strconv.FormatUint(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(plain))
}

// decodeAuditCursor unpacks an opaque cursor. Returns audit.ErrInvalidCursor
// on any parse failure so the REST edge can respond 400.
func decodeAuditCursor(cursor string) (time.Time, uint64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, audit.ErrInvalidCursor
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, audit.ErrInvalidCursor
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, audit.ErrInvalidCursor
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, audit.ErrInvalidCursor
	}
	return time.UnixMicro(micros).UTC(), id, nil
}
