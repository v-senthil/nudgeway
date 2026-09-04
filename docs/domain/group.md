# Domain: Group

## Purpose

A `Group` is a persisted, tenant-scoped mirror of a provider-side multi-party
conversation container — today the WhatsApp Business Cloud API's Groups
feature; the shape is provider-agnostic so a future channel (LINE, Messenger
group threads, Slack channel bridges) can reuse the same aggregate.

Groups are separate from `Contact`, `Session`, `Conversation`, `Message`, and
`Ticket`. In particular:

- A group message inbound webhook still creates a canonical `MessageReceived`
  event whose `from` is the participant's contact identity; the group id is
  carried as a sidecar so downstream fan-out can route it to the group inbox
  rather than a 1:1 chat.
- The roster (Members) is *not* the participant Contact list — a Member row
  can exist before the participant has ever messaged us (and therefore before
  we have a Contact resolved for them).

## Invariants

- `(OrgID, IntegrationID, ProviderGroupID)` is unique. The same provider-side
  group id under two different integrations is two different rows.
- `Subject` may be blank; provider allows a group with no subject.
- `Description` is optional; nullable in MySQL.
- `Size` is a hint from the provider's `total_participant_count`. The
  authoritative count is `SELECT COUNT(*) FROM group_members WHERE
  group_id = ? AND left_at IS NULL`.
- `IsAdmin` reports whether *our* business phone number holds admin rights
  in the group. All management-side calls (add/remove participants, update
  settings, reset invite link) require `IsAdmin = true`; a non-admin group
  is read-only from our POV.
- `Metadata` is a free-form bag for provider-native fields the domain does
  not model yet: `join_approval_mode`, `suspended`, `creation_timestamp`.
  Callers must not depend on any specific key being present.

Members:

- `(GroupID, WaID, BSUID)` is unique. A participant identified only by
  wa_id and a participant identified only by BSUID are two rows until we
  reconcile them via an inbound message that carries both.
- `ContactID` may be nil. It is populated lazily by the inbound pipeline
  once a message from the participant resolves them.
- `LeftAt` is the tombstone. Membership is soft-deleted so historical
  rosters can be reconstructed.

## Code references

- Domain type: `internal/domain/group/group.go` (`Group`, `Member`, `Role`)
- Errors: `internal/domain/group/errors.go`
- Repository port: `internal/ports/repository/groups.go` (`GroupRepo`)
- Application service: `internal/application/group/service.go` (`Service.Sync`,
  `List`, `Get`, `Members`, `SendMessage`)
- MySQL implementation: `internal/infrastructure/mysql/groups.go`
- Provider adapter surface: `internal/providers/whatsapp/groups.go`
  (`ListGroups`, `GetGroup`, `ListGroupMembers`) — satisfies
  `application/group.ProviderGroupsClient` structurally
- REST: `internal/api/rest/v1/groups.go` (`GET /api/v1/groups`,
  `POST /api/v1/groups/sync`, `GET /api/v1/groups/{id}`,
  `GET /api/v1/groups/{id}/members`, `POST /api/v1/groups/{id}/messages`)
- RBAC constants: `internal/domain/rbac/perms_groups.go`
  (`groups.read`, `groups.manage`)
- Migration: `migrations/20260904000004_groups.up.sql`,
  `migrations/20260904000004_grant_groups_perms.up.sql`
- Meta reference (source of truth): `~/Documents/whatsapp_doc_tracker/docs/groups/`

## Wire-up notes

The REST routes are installed via `v1.MountGroups(...)` — the parallel
router commit does not have to touch `router.go`. `cmd/server` calls this
after the standard `v1.Mount(...)` once it has built the reusable
middleware chain builders and constructed the `application/group.Service`
with a real `ProviderRegistry` (the same one wired into `SendService`).
