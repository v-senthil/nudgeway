// Package contact models the customer identity. A Contact may carry multiple
// ContactIdentity rows (phone, email, WhatsApp, BSUID, external CRM ID). The
// merge key is (org_id, provider, normalized_value).
package contact

import (
	"errors"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID uniquely identifies a Contact within an organization. Stored as
// VARBINARY(16) in MySQL; exchanged as ULID/UUIDv7 string across API + logs.
type ID string

// IdentityID references the primary identity associated with a Contact.
type IdentityID string

// Contact is the canonical customer aggregate. A Contact is provider-agnostic:
// it can be reached across any channel via one or more Identity rows.
type Contact struct {
	ID                ID
	OrgID             organization.ID
	DisplayName       string
	AvatarURL         string
	PrimaryIdentityID *IdentityID
	LastSeenAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ErrDisplayNameEmpty is returned when a Contact is created with a blank name
// and no fallback (e.g. WhatsApp profile name or phone tail) is provided.
var ErrDisplayNameEmpty = errors.New("contact: display name required")

// NormalizeDisplayName trims whitespace and collapses internal runs of
// spaces. Empty input returns ErrDisplayNameEmpty.
func NormalizeDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrDisplayNameEmpty
	}
	return strings.Join(strings.Fields(s), " "), nil
}

// Touch advances LastSeenAt. Callers persist the change.
func (c *Contact) Touch(now time.Time) {
	t := now.UTC()
	c.LastSeenAt = &t
}
