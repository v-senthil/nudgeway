package rbac

// PermAnalyticsRead gates read access to the analytics dashboard
// endpoints (GET /api/v1/analytics/overview, /series). Writes to the
// rollup tables are internal — the rollup worker runs off-request and
// does not consult RBAC.
//
// This constant lives in its own file so the base rbac.go's All() list
// stays untouched. The migration
// 20260904000007_grant_analytics_perms.up.sql idempotently backfills
// this permission onto every admin role at deploy time.
const PermAnalyticsRead Permission = "analytics.read"
