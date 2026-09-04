# Audit log

An `audit.Entry` row is an **append-only** record of a single tenant mutation: who did what, to which resource, when, and from which IP. The trail is the compliance and forensics backbone — every mutation path in the app layer emits an entry, and admins query the trail through `GET /api/v1/audit-logs`.

## Invariants

- Rows are write-once. There is no `UPDATE` or `DELETE` code path — retention pruning (Phase 4) drops rows older than the org's retention window.
- Every row carries an `org_id` — cross-tenant reads are physically impossible (`AuditRepo.List` scopes on org).
- `Action` values are stable, human-readable strings. Renaming a verb requires a migration because existing rows carry the old string.
- Writes never fail the caller. `application/audit.Service.Record` swallows repo errors after logging — an audit hiccup must never break a user mutation.
- Reads require the `audit.read` permission. Only admins get it by default (via `rbac.All()` seeded in `Bootstrap.EnsureAdminRole`).

## Fields

Defined in [`internal/domain/audit/entry.go`](../../internal/domain/audit/entry.go).

| Field | Notes |
|---|---|
| `ID uint64` | MySQL `BIGINT UNSIGNED AUTO_INCREMENT`. Rendered as decimal string on the wire. |
| `OrgID organization.ID` | Tenant boundary. Required. |
| `ActorUserID *user.ID` | Operator ULID; nil for system-driven mutations (workers, schedulers). |
| `Action Action` | Verb; one of the exported constants below. Required. |
| `ResourceType string` | Domain kind: `integration`, `message`, `conversation`, `attachment`, `session`. |
| `ResourceID string` | Affected row id; empty for bulk actions. |
| `IP net.IP` | Client IP recorded from the request. Nil for off-request writes. |
| `Metadata map[string]any` | Free-form JSON context (previous value on updates, message type on sends, etc.). |
| `OccurredAt time.Time` | Wall-clock at time of mutation. Server stamps when zero. |

## Actions (initial set)

Declared in [`internal/domain/audit/entry.go`](../../internal/domain/audit/entry.go). New actions land alongside the code that emits them.

| Constant | String value | Emitted from |
|---|---|---|
| `IntegrationCreated` | `integration.created` | `POST /api/v1/integrations` |
| `IntegrationDeleted` | `integration.deleted` | `DELETE /api/v1/integrations/{id}` |
| `IntegrationTested` | `integration.tested` | `POST /api/v1/integrations/{id}/test` |
| `MessageSent` | `message.sent` | `POST /api/v1/messages` |
| `MessageMarkedRead` | `message.marked_read` | `POST /api/v1/messages/{id}/read` |
| `ConversationMarkedRead` | `conversation.marked_read` | `POST /api/v1/conversations/{id}/read` |
| `AttachmentUploaded` | `attachment.uploaded` | `POST /api/v1/attachments` |
| `UserLoggedIn` | `user.logged_in` | `POST /api/v1/auth/login` |
| `UserLoggedOut` | `user.logged_out` | `POST /api/v1/auth/logout` |

## Query patterns

The MySQL implementation ([`internal/infrastructure/mysql/audit.go`](../../internal/infrastructure/mysql/audit.go)) drives three access paths, backed by the two indexes on `audit_logs`:

- **Newest-first tenant scan** — hits `KEY (org_id, occurred_at)`. Cursor pagination via `(occurred_at, id)` tuple encoded as opaque base64.
- **Per-resource history** — hits `KEY (org_id, resource_type, resource_id)`. Fill both `resource_type` and `resource_id` in the filter to get a single row's timeline.
- **Time-window scan** — combine `since` / `until` with the tenant scan. The primary index still drives the sort; MySQL filters the range in memory (bounded by the caller's `limit`).

## Storage

Table declared in [`migrations/20260903000001_organizations_users_roles.up.sql`](../../migrations/20260903000001_organizations_users_roles.up.sql) (lines 94–107). No new migration is required for this task.
