// Package identity is the sub-domain for ContactIdentity — the addressable
// value on a specific channel/provider for a Contact.
//
// Every provider that reaches a customer speaks in identities: phone, email,
// WhatsApp wa_id, BSUID, external CRM record ID. The (org, provider,
// normalized_value) tuple is the merge key used to deduplicate.
package identity

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID identifies a ContactIdentity row.
type ID string

// Type enumerates the kinds of identity values Nudgeway recognises.
type Type string

// Type values.
const (
	TypePhone    Type = "phone"     // E.164 phone number
	TypeEmail    Type = "email"     // RFC 5322 email
	TypeWhatsApp Type = "whatsapp"  // WhatsApp wa_id (digits, no plus)
	TypeBSUID    Type = "bsuid"     // Meta Business-Scoped User ID
	TypeExternal Type = "external"  // opaque external CRM ID
	TypeSocial   Type = "social"    // instagram/messenger/etc. account
)

// Identity is a Contact's addressable value on a specific channel + provider.
// The (OrgID, Provider, NormalizedValue) tuple is unique.
type Identity struct {
	ID              ID
	OrgID           organization.ID
	ContactID       contact.ID
	Type            Type
	Provider        string // e.g. "whatsapp", "zoho_desk", "email"
	Value           string // original as received
	NormalizedValue string // canonical form used for dedupe
	Verified        bool
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ErrPhoneInvalid is returned when a phone number cannot be normalized to
// E.164 by NormalizePhoneE164.
var ErrPhoneInvalid = errors.New("identity: invalid phone number")

// ErrEmailInvalid is returned when a value fails minimal email shape check.
var ErrEmailInvalid = errors.New("identity: invalid email")

// NormalizePhoneE164 produces the canonical E.164 form of a phone number:
// a leading '+' followed by 8-15 digits. Whitespace, dashes, parentheses,
// and dots are stripped. Values that do not start with '+' or a country
// digit are rejected. Full national-format parsing is delegated to
// libphonenumber later; this normalizer covers the WhatsApp wa_id case
// (digits only, no plus) by prepending '+'.
func NormalizePhoneE164(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrPhoneInvalid
	}
	var b strings.Builder
	b.Grow(len(raw) + 1)
	hasPlus := false
	if raw[0] == '+' {
		hasPlus = true
		raw = raw[1:]
	}
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		switch r {
		case ' ', '-', '(', ')', '.', '\t':
			continue
		default:
			return "", ErrPhoneInvalid
		}
	}
	digits := b.String()
	if l := len(digits); l < 8 || l > 15 {
		return "", ErrPhoneInvalid
	}
	_ = hasPlus // presence not required: WhatsApp wa_id has no plus
	return "+" + digits, nil
}

// NormalizeEmail lower-cases and trims. Full RFC validation is deferred.
func NormalizeEmail(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || !strings.Contains(raw, "@") || strings.Contains(raw, " ") {
		return "", ErrEmailInvalid
	}
	return raw, nil
}

// Normalize routes to the appropriate normalizer for the identity Type. For
// unknown types the raw value is returned trimmed.
func Normalize(t Type, raw string) (string, error) {
	switch t {
	case TypePhone, TypeWhatsApp:
		return NormalizePhoneE164(raw)
	case TypeEmail:
		return NormalizeEmail(raw)
	default:
		v := strings.TrimSpace(raw)
		if v == "" {
			return "", errors.New("identity: empty value")
		}
		return v, nil
	}
}
