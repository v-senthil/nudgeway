package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"
)

// SessionID is a base64url-encoded 32-byte opaque token.
type SessionID string

// Session is the platform-user login session persisted in MySQL.
type Session struct {
	ID         SessionID
	UserID     string
	OrgID      string
	IssuedAt   time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	IP         string
	UserAgent  string
}

// SessionStore persists sessions. Phase 0 ships an in-memory implementation
// for tests + walking-skeleton dev; Phase 0 Task 3 replaces it with a MySQL
// implementation behind this same interface.
type SessionStore interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, id SessionID) (Session, error)
	Touch(ctx context.Context, id SessionID, at time.Time) error
	Delete(ctx context.Context, id SessionID) error
}

// ErrSessionNotFound is returned when a session ID is unknown or expired.
var ErrSessionNotFound = errors.New("session not found")

// NewSessionID generates a cryptographically random session identifier.
func NewSessionID() (SessionID, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return SessionID(base64.RawURLEncoding.EncodeToString(b)), nil
}

// CookieOptions defines the cookie flags for session issuance.
type CookieOptions struct {
	Name     string
	Path     string
	Domain   string
	MaxAge   time.Duration
	Secure   bool
	SameSite http.SameSite
}

// SetSessionCookie writes the session cookie to the response with fullWA's
// mandated flags (HttpOnly, SameSite=Lax, Secure in prod).
func SetSessionCookie(w http.ResponseWriter, id SessionID, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     opts.Name,
		Value:    string(id),
		Path:     defaultStr(opts.Path, "/"),
		Domain:   opts.Domain,
		MaxAge:   int(opts.MaxAge.Seconds()),
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: nonZeroSameSite(opts.SameSite),
	})
}

// ClearSessionCookie writes an immediately-expiring version of the session cookie.
func ClearSessionCookie(w http.ResponseWriter, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     opts.Name,
		Value:    "",
		Path:     defaultStr(opts.Path, "/"),
		Domain:   opts.Domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: nonZeroSameSite(opts.SameSite),
	})
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func nonZeroSameSite(s http.SameSite) http.SameSite {
	if s == 0 {
		return http.SameSiteLaxMode
	}
	return s
}
