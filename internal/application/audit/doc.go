// Package audit is the application-layer service for the AuditLog trail.
//
// The service exposes two entry points:
//
//   - Record — called from every mutation path (integration create/delete,
//     message send, mark-as-read, attachment upload, login/logout) to
//     persist an audit row. The call is fire-and-forget from the caller's
//     perspective: failures are logged but never returned, so an audit
//     hiccup can never break the user request that triggered it.
//   - List — the read path powering GET /api/v1/audit-logs, returning a
//     paginated slice of Entry rows filtered by resource / actor / verb.
//
// Nothing in this package imports MySQL, HBase, Redis, or provider SDKs —
// persistence goes through repository.AuditRepo.
package audit
