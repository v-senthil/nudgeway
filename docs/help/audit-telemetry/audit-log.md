# Audit log

Append-only trail of every mutation performed inside the tenant. Rows are written from the application layer — integration lifecycle, message send / mark-read, attachment upload, login / logout, permission changes.

## Row shape

| Field | Notes |
|---|---|
| `id` | BIGINT primary key rendered as a decimal string. |
| `org_id` | Owning tenant ULID. |
| `actor_user_id` | Operator ULID. Empty for system-driven actions (rollup, scheduler). |
| `action` | Verb — `integration.created`, `message.sent`, `user.logged_in`, ... |
| `resource_type` | `integration | message | conversation | attachment | session` |
| `resource_id` | Affected entity id. Empty for bulk actions. |
| `ip` | Client IP (IPv4 or IPv6). |
| `metadata` | Free-form JSON — interpreted by the emitting caller. |
| `occurred_at` | RFC 3339. |

## How to use

1. Settings → Audit → **Audit log** tab.
2. Filter by any combination:
   - Resource type + resource id (requires both to hit the composite index).
   - Action (verb).
   - Actor (operator).
   - Time range (`since` / `until`).
3. Rows render newest-first, cursor-paginated.

## Action enum (partial)

Not enforced by DB — any string is legal — but the values written by the application today include:

- **Integrations**: `integration.created`, `integration.updated`, `integration.tested`, `integration.disconnected`, `integration.webhook_pushed`.
- **Messages**: `message.sent`, `message.marked_read`, `conversation.marked_read`.
- **Attachments**: `attachment.uploaded`.
- **Auth**: `user.logged_in`, `user.logged_out`, `session.expired`.
- **API tokens**: `api_token.created`, `api_token.revoked`.

Grep the codebase for new verbs: `grep -rn 'audit.Log(' internal/`.

## API

```
GET /api/v1/audit-logs
  ?resource_type=integration
  &resource_id=01J…
  &action=integration.disconnected
  &actor_user_id=01J…
  &since=2026-09-01T00:00:00Z
  &until=2026-09-05T00:00:00Z
  &cursor=eyJ…
  &limit=50
```

Response `200`:

```json
{
  "items": [
    {
      "id": "42713",
      "org_id": "01J…",
      "actor_user_id": "01J…",
      "action": "integration.disconnected",
      "resource_type": "integration",
      "resource_id": "01J…",
      "ip": "192.0.2.14",
      "metadata": { "reason": "operator_soft_disconnect" },
      "occurred_at": "2026-09-04T14:22:11Z"
    }
  ],
  "next_cursor": "eyJ…"
}
```

Filter semantics:

- `resource_id` **requires** `resource_type` (composite index).
- `since` is inclusive (`occurred_at >= since`).
- `until` is exclusive (`occurred_at < until`).
- Absent `next_cursor` in the response = you reached the end.

Requires `audit.read`. Additional statuses: `400` (invalid cursor / filter).

## MCP

| operationId | Purpose |
|---|---|
| `getAuditLogs` | List audit-log entries with filters + cursor pagination. |

## Troubleshooting

- **Filter returns nothing you expect** — `resource_id` without `resource_type` is silently ignored (composite index). Always pass both.
- **`400 invalid cursor`** — cursors are opaque and tied to the filter shape. If you change filters between requests, drop the cursor.
- **Missing rows you know should be there** — some code paths still emit slog-only. Grep for the code path and add an `audit.Log(...)` call if genuinely missing.

## Related

- [Audit & Meta telemetry overview](/#/audit-telemetry/overview)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
- [API token usage log](/#/api-tokens/usage-log-metrics)
