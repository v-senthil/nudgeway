package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	infauth "github.com/fullwa/fullwa/internal/infrastructure/auth"
)

// Sessions implements infrastructure/auth.SessionStore against web_sessions.
//
// The DB row key is sha256(SessionID)[:32] so the raw session token only
// ever lives inside the cookie itself. A stolen DB backup therefore cannot
// be used to hijack live sessions.
type Sessions struct {
	db *sql.DB
}

// NewSessions constructs a Sessions store.
func NewSessions(db *sql.DB) *Sessions { return &Sessions{db: db} }

// hash returns the DB row key for a session id.
func (s *Sessions) hash(id infauth.SessionID) []byte {
	sum := sha256.Sum256([]byte(id))
	return sum[:]
}

// ipBytes returns nil for empty/invalid, otherwise net.IP.To16() bytes.
func ipBytes(ip string) []byte {
	if ip == "" {
		return nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil
	}
	return parsed.To16()
}

// Create persists a new session row.
func (s *Sessions) Create(ctx context.Context, sess infauth.Session) error {
	uid, err := ulidToBytes(sess.UserID)
	if err != nil {
		return fmt.Errorf("session user_id: %w", err)
	}
	oid, err := ulidToBytes(sess.OrgID)
	if err != nil {
		return fmt.Errorf("session org_id: %w", err)
	}
	const q = `INSERT INTO web_sessions
	  (id, user_id, org_id, issued_at, last_seen_at, expires_at, ip, user_agent)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, q,
		s.hash(sess.ID), uid, oid,
		sess.IssuedAt.UTC(), sess.LastSeenAt.UTC(), sess.ExpiresAt.UTC(),
		ipBytes(sess.IP), sess.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("session insert: %w", err)
	}
	return nil
}

// Get returns the session or infauth.ErrSessionNotFound. Expired sessions
// are treated as absent.
func (s *Sessions) Get(ctx context.Context, id infauth.SessionID) (infauth.Session, error) {
	const q = `SELECT user_id, org_id, issued_at, last_seen_at, expires_at, ip, user_agent
	           FROM web_sessions WHERE id = ? LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, s.hash(id))
	var (
		uid, oid, ipRaw []byte
		issued, seen    time.Time
		expires         time.Time
		ua              string
	)
	if err := row.Scan(&uid, &oid, &issued, &seen, &expires, &ipRaw, &ua); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infauth.Session{}, infauth.ErrSessionNotFound
		}
		return infauth.Session{}, fmt.Errorf("session select: %w", err)
	}
	if time.Now().After(expires) {
		return infauth.Session{}, infauth.ErrSessionNotFound
	}
	uidStr, err := ulidFromBytes(uid)
	if err != nil {
		return infauth.Session{}, fmt.Errorf("session bad user_id: %w", err)
	}
	oidStr, err := ulidFromBytes(oid)
	if err != nil {
		return infauth.Session{}, fmt.Errorf("session bad org_id: %w", err)
	}
	ip := ""
	if len(ipRaw) > 0 {
		ip = net.IP(ipRaw).String()
	}
	return infauth.Session{
		ID: id, UserID: uidStr, OrgID: oidStr,
		IssuedAt: issued, LastSeenAt: seen, ExpiresAt: expires,
		IP: ip, UserAgent: ua,
	}, nil
}

// Touch updates last_seen_at.
func (s *Sessions) Touch(ctx context.Context, id infauth.SessionID, at time.Time) error {
	const q = `UPDATE web_sessions SET last_seen_at = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, at.UTC(), s.hash(id))
	if err != nil {
		return fmt.Errorf("session touch: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return infauth.ErrSessionNotFound
	}
	return nil
}

// Delete removes the session.
func (s *Sessions) Delete(ctx context.Context, id infauth.SessionID) error {
	const q = `DELETE FROM web_sessions WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, s.hash(id)); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}
