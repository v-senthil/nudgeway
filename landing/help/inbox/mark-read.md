# Mark messages as read

Marking an inbound message as read triggers WhatsApp's blue double-tick on the customer's phone. Nudgeway calls the channel adapter's mark-as-read API and stamps `read_at` locally. The inbox invokes this automatically when the operator opens a conversation — the REST + MCP surfaces are here for scripting.

## How to use

- **UI**: opening a conversation fires [`postConversationMarkRead`](#batch-mark-a-conversation) once, batching every unread inbound in the newest 50 messages.
- **Per-message**: rare — use when you want to mark one specific message but leave others unread (e.g. an assistant flow that only acknowledges one row).

## API

### Mark one message

**operationId**: `postMessageMarkRead`

```
POST /api/v1/messages/{id}/read
```

Outbound messages, messages without a `provider_message_id`, and messages already stamped `read_at` return `204` without side effects — idempotent by design.

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/messages/01M1…/read' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>'
```

### Batch mark a conversation

**operationId**: `postConversationMarkRead`

```
POST /api/v1/conversations/{id}/read
```

Iterates over the newest 50 messages in the conversation and calls the provider's mark-as-read for every inbound row with a `provider_message_id` that is not already stamped `read_at`. Partial failures are recorded server-side; the caller can retry.

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/conversations/01M1…/read' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>'
```

## MCP

- `postConversationMarkRead` — pass `{ "id": "<conv-ULID>" }`.
- `postMessageMarkRead` — pass `{ "id": "<message-ULID>" }`.

## Blue-tick semantics

- The blue tick appears on the customer's phone only when the customer has read receipts enabled (their setting, not yours).
- WhatsApp does not fire a `read` webhook back to Nudgeway when *the customer* reads *your* outbound — Meta only sends `read` for messages *you* mark. The delivered → read transition on your outbound bubbles comes from a separate `message_status` webhook the customer's client emits.
- Batching more than 50 inbounds requires paging: call the batch endpoint, load more history, call again.

## Troubleshooting

- **`204` returned but blue tick doesn't appear on the customer's phone** — the customer has read receipts turned off in WhatsApp Settings → Privacy. Nothing to fix on our end.
- **`424 message's integration cannot be resolved`** — the source integration was deleted after the message arrived. Reconnect and let the next inbound rebuild the link.
- **`502 upstream provider rejected the mark-as-read call`** — Meta rate limit or transient error. Nudgeway logs the details in the [Provider calls log](/#/audit-telemetry/provider-calls); retry after a short back-off.
- **Read state doesn't propagate to other operators' inboxes** — a WebSocket has dropped. Refresh; the `MessageRead` event should re-flow.

## Related

- [Conversation list](/#/inbox/conversations) — auto-marks read on open.
- [Send a text message](/#/inbox/send-text) — outbound send + delivery ticks.
- [Troubleshooting](/#/inbox/troubleshooting) — WebSocket + auth.
