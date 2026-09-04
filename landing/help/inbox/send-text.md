# Send a text message

The composer at the bottom of the middle pane is the primary send path for plain-text replies. Sends are optimistic — the row appears immediately, then reconciles when the send worker confirms.

## How to use

1. Click a conversation in the [list](/#/inbox/conversations).
2. Type in the composer.
3. Hit **Send** (or `Enter` — `Shift+Enter` inserts a newline).

The bubble renders with a spinning tick while queued, one grey tick on `sent`, two grey ticks on `delivered`, and two blue ticks on `read`. Each transition arrives via WebSocket from a `MessageSent` / `MessageDelivered` / `MessageRead` event.

## API

**operationId**: `postMessagesSend`

```
POST /api/v1/messages
```

Persists the message as `queued`, enqueues a job on the `message.send` lane, returns 202. The channel adapter is invoked asynchronously.

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/messages' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "conversation_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
    "type": "text",
    "text": { "body": "Your order shipped this morning." },
    "idempotency_key": "reply-01M1MC4KFJQ33YQWKPT7HKZNYC-1"
  }'
```

Response (`SendMessageAccepted`):

```json
{ "message_id": "01M1MC5X…", "status": "queued" }
```

Always pass `idempotency_key` — retries with the same key collapse to one Meta call. When omitted the server uses the message ID.

Set `text.preview_url: true` to let WhatsApp render a link preview for URLs in the body.

## MCP

Call the `postMessagesSend` tool with:

```json
{
  "body": {
    "conversation_id": "<conv-ULID>",
    "type": "text",
    "text": { "body": "<message>" },
    "idempotency_key": "<stable-key>"
  }
}
```

## Troubleshooting

- **`422` with message `outside 24h window`** — WhatsApp's customer-service window has expired. Send an approved [template](/#/inbox/send-template) instead.
- **`404 conversation not found`** — the ULID isn't in your org, or was resolved and archived. Re-list via [`getConversations`](/#/inbox/conversations).
- **`424 conversation has no configured integration`** — the integration was deleted or disabled. Reconnect via [Integrations](/#/integrations/connect-whatsapp).
- **Bubble stuck on spinning tick** — the send worker isn't consuming. `curl :8080/readyz` and check the worker logs for a Kafka lag warning.
- **Duplicate bubbles** — the frontend didn't reconcile because `idempotency_key` was missing. Always set it.

## Related

- [Send media](/#/inbox/send-media) — image / video / document flow.
- [Send a template message](/#/inbox/send-template) — outside the 24h window.
- [Mark messages as read](/#/inbox/mark-read) — blue-tick the customer's inbound.
