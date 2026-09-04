# Conversation list

The conversation list is the left pane of the inbox and the entry point for every reply. Rows are grouped by status (open / pending / resolved), sorted newest-activity-first, and enriched with the contact name, avatar, last-message preview, and unread count.

## How to use

- **Click a row** to load its thread into the middle pane. The client fires `getConversationMessages` (cursor-paginated, newest-first) and marks the conversation read via [`postConversationMarkRead`](/#/inbox/mark-read).
- **Filter chips** at the top of the pane narrow by status, assignee, or channel. Filters compose in the query string.
- **Search** hits contact name and phone; results scope to the current org automatically.

Group conversations render with `conversation.type === 'group'` and use the `subject` field instead of `contact_name` / `contact_avatar_url`.

## API

**operationId**: `getConversations`

```
GET /api/v1/conversations
```

```bash
curl -sS 'http://127.0.0.1:8080/api/v1/conversations' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>'
```

Response shape (`ConversationList`):

```json
{
  "items": [
    {
      "id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
      "contact_id": "01M1...",
      "session_id": "01M1...",
      "status": "open",
      "last_message_preview": "Your order shipped this morning.",
      "last_message_at": "2026-09-05T09:14:22Z",
      "unread_count": 0
    }
  ],
  "next_cursor": null
}
```

Load one thread:

```
GET /api/v1/conversations/{id}/messages
```

## MCP

Call the `getConversations` tool with no arguments to list the org's conversations. Follow up with `getConversationMessages` and the returned `id`.

## Troubleshooting

- **List empty even though the customer messaged you** — no integration is connected, or Meta hasn't been subscribed to `messages`. Check [Push webhook to Meta](/#/integrations/webhook-setup).
- **`401 unauthorized`** — session cookie expired. Re-login, or use a bearer API token for programmatic access.
- **`403 CSRF failure`** on `postConversationMarkRead` — the client didn't send the `X-CSRF-Token` header; hit `GET /api/v1/auth/csrf` first.
- **New messages don't appear live** — the `/ws/inbox` socket dropped. Check the DevTools Network tab; the client reconnects with backoff, then re-fetches the open thread.
- **Group conversations show a blank name** — expected. Use `subject`, not `contact_name`.

## Related

- [Inbox overview](/#/inbox/overview) — three-pane layout + WebSocket.
- [Send a text message](/#/inbox/send-text) — the composer's default path.
- [Mark messages as read](/#/inbox/mark-read) — blue-tick semantics.
