// Package audit contains the AuditLog domain entity — an append-only
// record of every mutation performed by a principal on behalf of an
// organization. Entries are written from the application layer through
// the repository.AuditRepo port; nothing in this package imports infra
// or provider code.
//
// Retention: entries are kept for the org's contractual retention window
// (7 years for regulated tenants). The domain package does not enforce
// retention itself; a scheduled job in internal/workers prunes older
// rows once the retention feature ships (Phase 4).
package audit
