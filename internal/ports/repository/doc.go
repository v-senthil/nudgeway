// Package repository holds the persistence port interfaces used by the
// application layer. Concrete implementations live in
// internal/infrastructure/mysql (for transactional entities) and
// internal/infrastructure/hbase (for high-volume append-only data).
//
// Every repository method takes a context and enforces org-scoping —
// there is no query surface that reads across tenants.
package repository
