// Package organization defines the tenant boundary. Every other domain
// entity carries an OrganizationID and every query is scoped by it.
//
// Full types land in Phase 0 (Organization, Settings) and expand in Phase 1
// with the RBAC wiring.
package organization
