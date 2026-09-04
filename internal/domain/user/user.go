package user

import (
	"errors"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID identifies a platform user (agent/admin).
type ID string

// Status enumerates user lifecycle states.
type Status string

// Status values.
const (
	StatusInvited  Status = "invited"
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// User represents a platform operator (agent, admin).
type User struct {
	ID           ID
	OrgID        organization.ID
	Email        string
	PasswordHash []byte // argon2id encoded (PHC string, stored as bytes)
	DisplayName  string
	Status       Status
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ErrEmailInvalid is returned when a normalized email fails minimal validation.
var ErrEmailInvalid = errors.New("invalid email")

// NormalizeEmail lower-cases and trims. Full RFC validation is deferred to
// the mailer / OpenAPI schema; this normalizer is the merge key.
func NormalizeEmail(e string) (string, error) {
	e = strings.TrimSpace(strings.ToLower(e))
	if e == "" || !strings.Contains(e, "@") || strings.Contains(e, " ") {
		return "", ErrEmailInvalid
	}
	return e, nil
}

// CanLogin reports whether a user is allowed to authenticate.
func (u User) CanLogin() bool { return u.Status == StatusActive }
