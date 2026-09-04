# Audit & Meta telemetry

Two operator surfaces that answer different questions:

- **Audit log** — *who did what inside Nudgeway?* Append-only rows written by every mutation path (integration lifecycle, message send / read, attachment upload, login / logout). Answers "who deleted this integration?", "who marked this conversation read?", "when did this user log in?".
- **Meta API execution log** — *what did our provider adapters actually send to Meta?* One row per outbound HTTP call, with request body, response body, status code, latency, and error class. Answers "why did that message fail?", "which endpoint returned 400?", "how many `send_message` calls did we make in the last hour?".

Both are cheap-to-read (indexed on `org_id + occurred_at`), tenant-scoped, and available over REST + MCP.

## Which one do I want?

| Question | Surface |
|---|---|
| Who added this WhatsApp integration? | [Audit log](/#/audit-telemetry/audit-log) filtered on `action=integration.created` |
| Why did that outbound message return 502? | [Meta API execution log](/#/audit-telemetry/provider-calls) filtered on `integration_id=…&operation=send_message` |
| When was the last login for user X? | Audit log filtered on `actor_user_id=…&action=user.logged_in` |
| Which Meta endpoint is currently rate-limiting us? | Meta API execution log filtered on `status_min=429` |

## Retention

Both logs are append-only. No TTL is enforced today — plan for offline archival before you hit disk budget on the raw `audit_logs` / `provider_calls` tables.

## Permissions

- **Audit log**: `audit.read`.
- **Meta API execution log**: `integrations.manage` (read + manage share the same key today).

## Pages

- [Audit log](/#/audit-telemetry/audit-log)
- [Meta API execution log](/#/audit-telemetry/provider-calls)

Source of truth: `docs/api-token-usage.md` (for the analogous per-token log), `internal/api/openapi/openapi.yaml`.
