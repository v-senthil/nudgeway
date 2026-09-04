# Outbound calls

Placing a call from the operator side is a single `initiateCall` invocation, gated on the recipient having granted call permission. The call queues, Meta rings the recipient's WhatsApp, and the resulting state transitions render as info messages in the thread just like inbound calls.

## Preconditions

- The recipient's call permission for this integration is `permanent`, or `temporary` and unexpired.
- The current user has the `calls.manage` permission.
- The integration is connected + tested.

Check the permission first — see [Call permissions](/#/calls/call-permissions).

## How to use

1. Open the contact's conversation.
2. Right-pane → **Call** button. The button reflects the permission state (`permanent` / `temporary` / `no_permission`) — disabled with a `Request permission` chip when it's `no_permission`.
3. Click **Call**. The call queues, then rings the recipient.
4. On answer, the browser tab hosts the WebRTC session (same as inbound — mic access required).

## API

**operationId**: `initiateCall`

```
POST /api/v1/calls
```

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/calls' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
    "to": "918197002143",
    "contact_id": "01M1…",
    "idempotency_key": "call-01M1…",
    "recording": { "enabled": true },
    "transcription": { "enabled": true, "language": "en" }
  }'
```

- **`to`** — E.164 without the leading `+`.
- **`to_user_id`** — alternative to `to` when you have a BSUID (business-scoped user id).
- **`recording`** / **`transcription`** — optional; Meta stores + Nudgeway proxies. See [Recording + transcript](/#/calls/recording-transcript).

Response (`InitiateCallAccepted`):

```json
{ "call_id": "01M1…", "status": "queued" }
```

Status transitions arrive via `CallRinging` / `CallAnswered` / `CallCompleted` events over the WebSocket.

## MCP

Call the `initiateCall` tool with the same body shape. Precede with `getIntegrationCallPermission` when you're not sure the recipient has granted permission.

## Troubleshooting

- **`400 validation error`** — usually a bad `to` (had a leading `+`, contained spaces). Send digits only.
- **`403 missing calls.manage`** — role doesn't grant call management. Admin can grant.
- **`424 integration missing`** — the integration was deleted or disabled. Reconnect.
- **`502 provider error`** — Meta rejected inline. The most common cause is `no_permission` on the recipient's user — call [`getIntegrationCallPermission`](/#/calls/call-permissions) first, or send a [`sendCallPermissionRequest`](/#/calls/call-permissions).
- **Call goes to `no_answer` immediately** — recipient's WhatsApp had the call channel disabled, or the phone was off. Nothing to fix on our end; try again later.

## Related

- [Call permissions](/#/calls/call-permissions) — check + request before you can call.
- [Inbound calls](/#/calls/inbound-call) — the popup + WebRTC accept flow.
- [Recording + transcript](/#/calls/recording-transcript) — post-call artefacts.
