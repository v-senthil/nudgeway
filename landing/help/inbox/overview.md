# Inbox overview

The Inbox is a real-time, three-pane surface for every WhatsApp conversation your org owns. Left pane lists conversations, middle pane renders the selected thread with WhatsApp-native bubbles, right pane shows contact + session context. Every pane is org-scoped by the session cookie — cross-tenant reads are impossible from the UI or the API.

## The three panes

- **Left — Conversation list.** Fetched via [`getConversations`](/#/inbox/conversations); grouped by status (open / pending / resolved). Filter by contact, session, or the assigned agent.
- **Middle — Thread + composer.** Loaded via `getConversationMessages`, cursor-paginated newest-first. The composer sends text, media, or approved templates via [`postMessagesSend`](/#/inbox/send-text).
- **Right — Contact + session drawer.** Contact profile, recent orders / tickets (if a CRM integration is wired), and the raw session metadata.

## Real-time via WebSocket

The inbox subscribes to `wss://<host>/ws/inbox` on load. Every canonical event that touches your org (`MessageInbound`, `MessageSent`, `MessageDelivered`, `MessageRead`, `MessageFailed`, `CallRinging`, `CallAnswered`, ...) is pushed to the socket and reduced into the store. New messages appear without a refresh; delivery ticks flip in real time; incoming calls raise the [Inbound call popup](/#/calls/inbound-call).

Reconnect is automatic with exponential backoff. On reconnect the client re-fetches the currently open conversation to fill any gap.

## Domain rules

`Contact ≠ Session ≠ Conversation ≠ Message ≠ Ticket` — treat them as distinct first-class entities. One contact can have many sessions; one session can have many conversations; one conversation has many messages. The inbox surfaces conversations; drill through to sessions or tickets from the right pane.

## What ships with the inbox

| Feature | Page |
|---|---|
| List + filter conversations | [Conversation list](/#/inbox/conversations) |
| Send a text reply | [Send a text message](/#/inbox/send-text) |
| Send image / video / document | [Send media](/#/inbox/send-media) |
| Send a template (outside 24h window) | [Send a template message](/#/inbox/send-template) |
| Blue-tick outstanding inbound | [Mark messages as read](/#/inbox/mark-read) |
| Common failure modes | [Troubleshooting](/#/inbox/troubleshooting) |

## Related

- [Templates overview](/#/templates/overview) — required for replies outside the 24h window.
- [Calls overview](/#/calls/overview) — call state transitions render as info messages in the same thread.
- [Connect a WhatsApp integration](/#/integrations/connect-whatsapp) — nothing flows until an integration exists.
