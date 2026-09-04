package rbac

// Groups permission constants. Declared in their own file so adding a new
// resource does not touch the seed catalogue in rbac.go. The
// 20260904000004_grant_groups_perms migration idempotently backfills these
// onto existing admin roles so operators see the new pages the moment the
// binary boots against a pre-groups database.
const (
	// PermGroupsRead gates listing + reading Group entities and their
	// members. Auto-granted to admin roles by the migration.
	PermGroupsRead Permission = "groups.read"

	// PermGroupsManage gates syncing groups from the provider, sending
	// messages to groups, and any future mutation surface (create, remove
	// participants, update settings, ...). Auto-granted to admin roles.
	PermGroupsManage Permission = "groups.manage"
)
