# Audit & Meta telemetry

Two operator surfaces under **Settings -> Audit** that answer different questions:

- **Audit log** — *who did what inside your workspace?* Every action a person or the system took: added an integration, marked a conversation read, uploaded an attachment, logged in. Use this for "who deleted this integration?" or "when did user X last sign in?".
- **Meta API execution log** — *what did we actually send to Meta, and what did Meta reply?* One row per outbound HTTP call to Meta, including the exact request, the exact response, the status code, and the latency. Use this for "why did that message fail?" or "which endpoint is being rate-limited?".

Both are append-only, scoped to your workspace, and readable in the browser.

## Which one do I want?

| Question | Surface |
|---|---|
| Who added this WhatsApp integration? | [Audit log](#/audit-telemetry/audit-log), filter action = `integration.created` |
| Why did that outbound message fail? | [Meta API execution log](#/audit-telemetry/provider-calls), filter operation = `send_message` |
| When did user X last log in? | Audit log, filter action = `user.logged_in` |
| Which Meta endpoint is throttling us? | Meta API execution log, filter status ≥ 429 |

## Retention

Both logs grow forever today; nothing is automatically deleted. Plan for offline archival if you handle very high volumes.

## Permissions

- **Audit log**: `audit.read`.
- **Meta API execution log**: `integrations.manage`.

Ask an org admin to grant these if the tabs don't appear for you.

## Pages

- [Audit log](#/audit-telemetry/audit-log)
- [Meta API execution log](#/audit-telemetry/provider-calls)
