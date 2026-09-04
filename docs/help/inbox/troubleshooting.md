# Inbox troubleshooting

The common inbox failure modes and the one-line fix for each. For anything not here, check the [Provider calls log](/#/audit-telemetry/provider-calls) — every Meta invocation is recorded with the request + response.

## 24-hour window

- **`422 outside 24h window`** on `postMessagesSend` with `type: text` — WhatsApp allows plain sends only within 24 hours of the customer's last inbound. Switch to a [template](/#/inbox/send-template).
- **Composer stays greyed out even after a fresh inbound** — the frontend uses `session.last_inbound_at` to decide; if the WebSocket dropped it may not have refreshed. Reload.
- **Template composer says "no approved templates"** — the integration has no `APPROVED` templates for its default language. Run [Sync from Meta](/#/templates/sync-from-meta) or [Create a template](/#/templates/create).

## Authentication + CSRF

- **`401 not authenticated`** — session cookie expired (default 24h). Log in again, or use a bearer API token.
- **`403 CSRF failure`** on any `POST` — the client didn't send the `X-CSRF-Token` header. Fetch `GET /api/v1/auth/csrf` first, or use a bearer token (which skips CSRF).
- **`403 missing permission`** — the user's role doesn't grant `messages.send`. An admin can grant it under Settings → Users.

## WebSocket disconnect

- **Live updates stop; no error visible** — check DevTools Network for a dropped `wss://…/ws/inbox`. The client reconnects with exponential backoff (1s, 2s, 4s, ...). On reconnect it re-fetches the open thread to fill the gap.
- **Frequent disconnects behind a corporate proxy** — many proxies close idle WebSockets after 60s. Nudgeway sends ping frames every 30s by default; if the proxy still kills the socket, whitelist the path.
- **Delivery ticks never advance past one grey tick** — the send worker is running but the `messages/status` webhook isn't reaching you. Re-check [Push webhook to Meta](/#/integrations/webhook-setup) and Meta's subscriptions.

## Attachment upload fail

- **`413 Payload Too Large`** — file > 16 MiB. Compress or split; resumable uploads land in a later phase.
- **Upload returns 201 but send returns `424` immediately after** — the conversation's integration was deleted between the two calls. Reconnect via [Integrations](/#/integrations/connect-whatsapp).
- **Meta returns `media_not_reachable`** — Meta couldn't fetch the URL you handed it. Your public tunnel is down; restart `cloudflared` / `ngrok`.
- **Wrong MIME on Meta's side** — pass an explicit `media.mime_type` on the send call; Meta trusts the header over the URL extension.

## General

- **`424` on a send that used to work** — the target conversation's integration was disabled or its access token expired. Run [Test the connection](/#/integrations/test-connection).
- **Duplicate bubbles in the thread** — the frontend couldn't reconcile the optimistic row. Always pass an `idempotency_key` on `postMessagesSend`.

## Related

- [Inbox overview](/#/inbox/overview) — three-pane layout, WebSocket.
- [Provider calls log](/#/audit-telemetry/provider-calls) — every Meta invocation, with request + response.
- [Integrations](/#/integrations/overview) — reconnect + rotate credentials.
