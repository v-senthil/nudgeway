package rbac

// Call-related permission constants. These do NOT appear in rbac.All()
// intentionally — the admin seed backfill migration
// (20260904000005_grant_calls_perms) grants both keys to existing admin
// roles so we don't need to churn the initial seed catalogue.
const (
	// PermCallsRead gates read access to the /api/v1/calls index + drill-down
	// + recording proxy.
	PermCallsRead Permission = "calls.read"

	// PermCallsManage gates mutation of calls: initiate outbound, answer /
	// reject / end. Also gates the recording proxy write path when a
	// signed-URL refresh is required.
	PermCallsManage Permission = "calls.manage"
)
