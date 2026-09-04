# Send to a group

`sendGroupMessage` enqueues an outbound text, template, or media message addressed to a group. Same canonical shape as [Send a text message](/#/inbox/send-text) — the request is persisted `queued`, a `message.send` job fires, the provider adapter runs asynchronously, and status transitions (`sent`, `delivered`, `read`, `failed`) arrive via events + Meta status webhooks.

## How to use

From the Inbox, open the group thread and use the composer exactly the way you do for a 1:1 chat. Under the hood the composer routes to `POST /api/v1/groups/{id}/messages` instead of `POST /api/v1/messages`.

## API

```
POST /api/v1/groups/{id}/messages
Content-Type: application/json

{
  "type": "text",
  "text": { "body": "Reminder: standup in 10 minutes." },
  "idempotency_key": "01J…"
}
```

Response `202`:

```json
{ "message_id": "01J…", "status": "queued" }
```

Supported `type` values: `text | template | image | video | audio | document | sticker`. Bodies mirror the 1:1 shapes:

- `text` → `{"text": {"body": "..."}}`
- `template` → `{"template": {...}}` (see [Send a template message](/#/inbox/send-template))
- `image | video | audio | document | sticker` → `{"media": {"media_id": "..."}}` or `{"media": {"url": "..."}}`

Requires CSRF + `messages.send`. Additional statuses: `403` (missing permission), `404` (group not found), `502` (Meta rejected the send).

## MCP

| operationId | Purpose |
|---|---|
| `sendGroupMessage` | Enqueue an outbound message to a group. |

## Ordering

Kafka partitions on `conversation_id`, so per-group ordering is guaranteed by the send worker. If you send two texts back-to-back, they arrive at Meta in that order.

## Troubleshooting

- **`404 not found`** — the ULID does not resolve, or the group belongs to another org. Verify via `GET /api/v1/groups/{id}`.
- **`502 provider_error`** — Meta rejected the send. Look up the exact request/response in [Meta API execution log](/#/audit-telemetry/provider-calls) filtered on `operation=send_message`.
- **Non-admin group** — sending text works on any group Meta returns; management calls (invite reset, add participant) require `is_admin: true`.
- **24-hour window** — same rule as 1:1: free-form text needs an open customer-service window. Use a template otherwise.

## Related

- [Groups overview](/#/groups/overview)
- [List + sync groups](/#/groups/list-sync)
- [Send a template message](/#/inbox/send-template)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
