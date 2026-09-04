package apitoken

import (
	"errors"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/user"
)

// ID is the canonical identifier of an api_tokens row (ULID string form).
type ID string

// Token is the persisted representation of a long-lived API token.
//
// The plaintext token is never stored — only Prefix (indexed, shown in the
// UI) plus SecretHash (argon2id-encoded) live on disk. The plaintext is
// returned to the caller exactly once at creation time.
type Token struct {
	ID         ID
	OrgID      organization.ID
	UserID     user.ID
	Name       string
	Prefix     string
	SecretHash []byte
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// NewID returns a fresh ULID-shaped ID suitable for a new api_tokens row.
func NewID() ID { return ID(ulid.Make().String()) }

// ErrNotFound is returned when a lookup finds no matching token.
var ErrNotFound = errors.New("api token not found")

// ErrRevoked is returned when the caller attempts to use a revoked token.
var ErrRevoked = errors.New("api token revoked")

// ErrExpired is returned when the caller attempts to use an expired token.
var ErrExpired = errors.New("api token expired")

// Active reports whether the token is currently usable at the given
// instant — i.e. not revoked and not past its optional expiry.
func (t Token) Active(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return false
	}
	return true
}
