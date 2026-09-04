// Package group models the WhatsApp Business Cloud API "group" concept as a
// provider-agnostic domain entity.
//
// A Group is a persisted, org-scoped mirror of a provider-side group instance
// (today WhatsApp; the shape is deliberately provider-neutral). Membership is
// tracked as a separate first-class collection of Member rows — a participant
// may exist in the roster before we have resolved them to a full Contact.
//
// Invariants:
//   - (OrgID, IntegrationID, ProviderGroupID) is unique. The same group id
//     from two different integrations is two different Group rows.
//   - Subject is human-visible and may be blank; Description is optional.
//   - Size is a hint only. The authoritative participant count is the number
//     of Member rows with LeftAt = nil.
//   - IsAdmin reports whether OUR business phone number holds admin rights
//     in the group. Group-level management calls require IsAdmin = true.
//   - Metadata is a free-form bag for provider-native fields the domain does
//     not yet model (join_approval_mode, suspended, creation_timestamp).
package group

import (
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// ID uniquely identifies a Group within an organization. Stored as
// VARBINARY(16) in MySQL; exchanged as a ULID string across API + logs.
type ID string

// Role classifies the authority level of a Member inside a Group.
type Role string

// Role values.
const (
	// RoleMember is a regular participant with no admin rights.
	RoleMember Role = "member"

	// RoleAdmin can add / remove participants and update group settings.
	RoleAdmin Role = "admin"

	// RoleSuperAdmin (aka group creator on the provider side) is the seed
	// admin who created the group. Cannot be demoted by other admins.
	RoleSuperAdmin Role = "superadmin"
)

// Group is the canonical group aggregate. Provider-agnostic — a Group could
// in principle be surfaced by any channel provider that models multi-party
// conversations, not just WhatsApp.
type Group struct {
	ID              ID
	OrgID           organization.ID
	IntegrationID   integration.ID
	ProviderGroupID string
	Subject         string
	Description     string
	Size            int
	IsAdmin         bool
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Member is one participant row. ContactID may be nil when the participant
// has not yet sent an inbound message that would resolve them to a Contact.
// The pair (WaID, BSUID) is the provider-native handle we identify them by
// until that resolution happens.
type Member struct {
	// ID is the row auto-increment id. Zero on values that have not yet
	// been persisted.
	ID uint64

	// OrgID is the tenant boundary. Required.
	OrgID organization.ID

	// GroupID is the parent Group.ID.
	GroupID ID

	// ContactID is the resolved Contact, if any.
	ContactID *contact.ID

	// WaID is the participant's WhatsApp phone number (E.164 without '+').
	// Empty when the provider only surfaced a BSUID.
	WaID string

	// BSUID is the business-scoped user id. Empty when the provider only
	// surfaced a wa_id.
	BSUID string

	// Role classifies member vs admin vs superadmin.
	Role Role

	// JoinedAt is when we first observed the member in the group.
	JoinedAt time.Time

	// LeftAt is set when the member is removed / leaves. Nil means active.
	LeftAt *time.Time
}
