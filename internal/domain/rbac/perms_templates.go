package rbac

// Template management permissions.
//
// Kept in a separate file so the wire-up commit that adds them to All()
// does not conflict with parallel feature work adding sibling permissions
// (calls, groups, analytics). The migration
// 20260904000004_grant_templates_perms grants both to every existing
// admin role.
const (
	// PermTemplatesRead gates read access to the templates library
	// (GET /api/v1/templates and friends).
	PermTemplatesRead Permission = "templates.read"
	// PermTemplatesManage gates create / edit / submit / sync / delete of
	// templates (POST/PUT/DELETE /api/v1/templates*).
	PermTemplatesManage Permission = "templates.manage"
)
