---
name: nudgeway-inbox
description: List and manipulate WhatsApp conversations — fetch inbox, load a thread, send outbound messages (text, media, template), mark inbound as read. Multi-tenant, org-scoped by the session.
trigger: User asks about the WhatsApp inbox, conversations, sending messages, marking messages read, or looking up a thread by contact.
---

# Nudgeway inbox skill

## Overview

The inbox is a real-time, WebSocket-backed three-pane surface. Every conversation belongs to one contact (or one WhatsApp group). Every message belongs to one conversation. The domain enforces the invariant Contact ≠ Session ≠ Conversation ≠ Message — treat them as distinct.

## MCP tools

| operationId | Purpose |
|---|---|
| `getConversations` | List open/pending/resolved conversations for the caller's org. |
| `getConversationMessages` | Load the full thread for one conversation (newest first). |
| `postMessagesSend` | Send an outbound message — text, media, or template. Idempotent via `client_reference_id`. |
| `postConversationMarkRead` | Mark every inbound message in a conversation as read (blue tick to the customer). |
| `postMessageMarkRead` | Mark one specific inbound message as read. |
| `postAttachmentsUpload` | Upload a media blob (used before sending an image/video/document). |

## REST equivalents

```
GET  /api/v1/conversations
GET  /api/v1/conversations/{id}/messages
POST /api/v1/messages
POST /api/v1/conversations/{id}/read
POST /api/v1/messages/{id}/read
POST /api/v1/attachments
```

## Patterns

### Send a text reply

```json
{
  "tool": "postMessagesSend",
  "arguments": {
    "body": {
      "conversation_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
      "type": "text",
      "text": "Your order shipped this morning.",
      "client_reference_id": "reply-01M1..."
    }
  }
}
```

### Send a media message (two-step)

```json
// 1. Upload the file → returns { url, media_id }
{ "tool": "postAttachmentsUpload", "arguments": { "body": <multipart> } }

// 2. Send referencing the media_id (preferred) or url
{ "tool": "postMessagesSend", "arguments": { "body": {
    "conversation_id": "...",
    "type": "image",
    "media": { "media_id": "852743...", "caption": "Latest catalogue" },
    "client_reference_id": "..."
}}}
```

### Mark a conversation read when the operator opens it

```json
{ "tool": "postConversationMarkRead", "arguments": { "id": "<conv-ULID>" } }
```

## Gotchas

- **24-hour window**: WhatsApp enforces a 24-hour reply window from the customer's last inbound. Outside that window, only pre-approved templates can be sent — a plain text send will 422.
- **Optimistic UI**: the frontend echoes messages before the send-worker confirms. Always pass `client_reference_id` so the caller can reconcile the optimistic row with the accepted row.
- **Group conversations**: `conversation.type === 'group'` — some fields (contact_name, contact_avatar_url) will be empty; use `subject` instead.

## Related skills

- [`nudgeway-templates`](../nudgeway-templates/SKILL.md) — when replying outside the 24h window.
- [`nudgeway-integrations`](../nudgeway-integrations/SKILL.md) — before the first message can flow, an integration must exist + be tested.
