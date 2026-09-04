package rbac

// Permission is a resource.action key, e.g. "contacts.read", "messages.send".
// The full permission catalogue is enumerated per feature area; this file
// carries only the strongly-typed alias so packages don't scatter untyped strings.
type Permission string

// Well-known permission constants used by Phase 0. Later phases add more.
const (
	// System / self.
	PermUsersRead      Permission = "users.read"
	PermUsersManage    Permission = "users.manage"
	PermRolesManage    Permission = "roles.manage"

	// Placeholder domain permissions used only for arch tests until the
	// corresponding endpoints land.
	PermContactsRead     Permission = "contacts.read"
	PermMessagesSend     Permission = "messages.send"
	PermIntegrationsManage Permission = "integrations.manage"
	// PermAuditRead gates read access to the audit trail
	// (GET /api/v1/audit-logs). Writes to audit_logs are internal only.
	PermAuditRead Permission = "audit.read"
)

// All returns every declared Permission constant. It is the authoritative
// list used when seeding the built-in admin role.
func All() []Permission {
	return []Permission{
		PermUsersRead,
		PermUsersManage,
		PermRolesManage,
		PermContactsRead,
		PermMessagesSend,
		PermIntegrationsManage,
		PermAuditRead,
		PermTemplatesRead,
		PermTemplatesManage,
		PermGroupsRead,
		PermGroupsManage,
		PermCallsRead,
		PermCallsManage,
		PermAnalyticsRead,
	}
}

// PermissionSet is an unordered set of permissions granted to a principal.
type PermissionSet map[Permission]struct{}

// NewSet builds a PermissionSet from a slice.
func NewSet(perms ...Permission) PermissionSet {
	s := PermissionSet{}
	for _, p := range perms {
		s[p] = struct{}{}
	}
	return s
}

// Has reports whether the set contains p.
func (s PermissionSet) Has(p Permission) bool {
	if s == nil {
		return false
	}
	_, ok := s[p]
	return ok
}

// Add merges permissions into the set (in place).
func (s PermissionSet) Add(perms ...Permission) {
	for _, p := range perms {
		s[p] = struct{}{}
	}
}
