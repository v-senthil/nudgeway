package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
)

// SessionsComm implements repository.SessionRepo against sessions_comm.
type SessionsComm struct {
	db *sql.DB
}

// NewSessionsComm constructs a SessionsComm repository.
func NewSessionsComm(db *sql.DB) *SessionsComm { return &SessionsComm{db: db} }

// FindOrCreateActive returns the single ACTIVE session for the
// (org, endpoint, contact) tuple, creating one if none exists. It relies
// on the UNIQUE(org_id, business_endpoint_id, active_contact_id) index
// backed by the STORED GENERATED active_contact_id column to make the
// insert-or-return atomic under concurrency.
func (s *SessionsComm) FindOrCreateActive(
	ctx context.Context,
	orgID organization.ID,
	endpointID session.BusinessEndpointID,
	contactID contact.ID,
) (session.Session, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions org: %w", err)
	}
	epBytes, err := ulidToBytes(string(endpointID))
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions endpoint: %w", err)
	}
	ctBytes, err := ulidToBytes(string(contactID))
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions contact: %w", err)
	}
	// Fast path: read the existing active row.
	if got, err := s.selectActive(ctx, orgBytes, epBytes, ctBytes); err == nil {
		return got, nil
	} else if !errors.Is(err, ErrNotFound) {
		return session.Session{}, err
	}
	// Try insert. The UNIQUE index makes concurrent inserts safe.
	newID := newULID()
	const insertQ = `INSERT INTO sessions_comm
	    (id, org_id, contact_id, business_endpoint_id, status, metadata)
	  VALUES (?, ?, ?, ?, 'active', JSON_OBJECT())`
	_, err = s.db.ExecContext(ctx, insertQ, newID[:], orgBytes, ctBytes, epBytes)
	if err != nil {
		// If we lost the race, fall through to the reread below.
		if !isDuplicateErr(err) {
			return session.Session{}, fmt.Errorf("sessions insert: %w", err)
		}
	}
	got, err := s.selectActive(ctx, orgBytes, epBytes, ctBytes)
	if err != nil {
		return session.Session{}, err
	}
	return got, nil
}

// Get fetches a session by (OrgID, ID).
func (s *SessionsComm) Get(ctx context.Context, orgID organization.ID, id session.ID) (session.Session, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions id: %w", err)
	}
	const q = `SELECT id, org_id, contact_id, business_endpoint_id, status, opened_at, closed_at, metadata
	           FROM sessions_comm WHERE org_id = ? AND id = ? LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, orgBytes, idBytes)
	return scanSession(row.Scan)
}

// Close transitions a session to closed. Idempotent on already-closed rows.
func (s *SessionsComm) Close(ctx context.Context, orgID organization.ID, id session.ID) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("sessions org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("sessions id: %w", err)
	}
	const q = `UPDATE sessions_comm SET status = 'closed', closed_at = ?
	           WHERE org_id = ? AND id = ? AND status = 'active'`
	if _, err := s.db.ExecContext(ctx, q, time.Now().UTC(), orgBytes, idBytes); err != nil {
		return fmt.Errorf("sessions close: %w", err)
	}
	return nil
}

// selectActive reads the current active session for the tuple.
func (s *SessionsComm) selectActive(ctx context.Context, orgBytes, epBytes, ctBytes []byte) (session.Session, error) {
	const q = `SELECT id, org_id, contact_id, business_endpoint_id, status, opened_at, closed_at, metadata
	           FROM sessions_comm
	           WHERE org_id = ? AND business_endpoint_id = ? AND active_contact_id = ? LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, orgBytes, epBytes, ctBytes)
	return scanSession(row.Scan)
}

// scanSession decodes a row into session.Session.
func scanSession(scan func(dest ...any) error) (session.Session, error) {
	var (
		id, org, ct, ep []byte
		status          string
		opened          time.Time
		closed          sql.NullTime
		metaBytes       []byte
	)
	if err := scan(&id, &org, &ct, &ep, &status, &opened, &closed, &metaBytes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.Session{}, ErrNotFound
		}
		return session.Session{}, fmt.Errorf("sessions scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions bad org: %w", err)
	}
	ctStr, err := ulidFromBytes(ct)
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions bad contact: %w", err)
	}
	epStr, err := ulidFromBytes(ep)
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions bad endpoint: %w", err)
	}
	out := session.Session{
		ID:                 session.ID(idStr),
		OrgID:              organization.ID(orgStr),
		ContactID:          contact.ID(ctStr),
		BusinessEndpointID: session.BusinessEndpointID(epStr),
		Status:             session.Status(status),
		OpenedAt:           opened,
	}
	if closed.Valid {
		t := closed.Time
		out.ClosedAt = &t
	}
	if len(metaBytes) > 0 {
		var m map[string]any
		if err := json.Unmarshal(metaBytes, &m); err != nil {
			return session.Session{}, fmt.Errorf("sessions metadata: %w", err)
		}
		out.Metadata = m
	}
	return out, nil
}
