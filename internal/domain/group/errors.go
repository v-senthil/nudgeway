package group

import "errors"

// ErrNotFound is returned when a Group / Member lookup does not resolve to a
// row for the caller's org. The infrastructure layer wraps this so callers
// can errors.Is on the sentinel without importing mysql.
var ErrNotFound = errors.New("group: not found")

// ErrIntegrationMissing is returned when a Sync call cannot resolve the
// (org, integration) pair to a live channel provider adapter.
var ErrIntegrationMissing = errors.New("group: integration missing")

// ErrInvalidRole is returned when a caller supplies a Role string that is
// not one of the three canonical values.
var ErrInvalidRole = errors.New("group: invalid role")

// ValidRole reports whether r is one of the canonical Role constants.
func ValidRole(r Role) bool {
	switch r {
	case RoleMember, RoleAdmin, RoleSuperAdmin:
		return true
	}
	return false
}
