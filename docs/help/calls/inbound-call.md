# Inbound calls (popup + WebRTC accept)

When a WhatsApp user calls your business number, Meta fires a `calls` webhook with a `connect` event carrying an SDP offer. Nudgeway persists the call as `ringing` and pushes a `CallRinging` event to every open inbox tab. The client renders a bottom-right popup with Accept / Reject.

## The popup

- **Accept** — starts WebRTC negotiation (below).
- **Reject** — hits `rejectCall`, Meta signals `declined` to the caller.
- **Auto-dismiss** — if the caller hangs up first, Meta fires `terminate` and the popup closes on the `CallCompleted` / `CallMissed` event.

Multiple tabs open? Only the tab that clicks Accept wins the SDP negotiation; the other tabs' popups close automatically when the call status flips off `ringing`.

## WebRTC negotiation

On Accept:

1. Client fetches the SDP offer.

   ```
   GET /api/v1/calls/{id}/session
   ```

   Returns `{ "sdp_type": "offer", "sdp": "<blob>" }`. Returns `404` when no offer is stored (rare — usually a webhook ordering issue).

2. Client builds an `RTCPeerConnection`, `setRemoteDescription(offer)`, generates `createAnswer()`, `setLocalDescription(answer)`.

3. Client POSTs the answer.

   ```
   POST /api/v1/calls/{id}/answer
   Content-Type: application/json

   { "sdp": "<answer sdp>" }
   ```

4. Server strips Chrome-only mDNS candidates and any `a=ice-options:trickle` line from the SDP before forwarding to Meta.

5. ICE gathers on both sides; audio starts.

If `body.sdp` is omitted, the call is answered without SDP negotiation — used only when the browser isn't the endpoint (e.g. a PSTN bridge, not yet shipped).

## API

- **operationId**: `answerCall` — `POST /api/v1/calls/{id}/answer`
- **operationId**: `rejectCall` — `POST /api/v1/calls/{id}/reject` with optional `{ "reason": "..." }`
- **operationId**: `endCall` — `POST /api/v1/calls/{id}/end` to hang up an in-progress call
- **operationId**: `getCallSession` — `GET /api/v1/calls/{id}/session` to fetch the stored offer

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/calls/01M1…/reject' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{"reason":"agent_unavailable"}'
```

## MCP

- `answerCall` — `{ "id": "<call-ULID>", "body": { "sdp": "..." } }` (SDP typically negotiated in-browser, not via MCP).
- `rejectCall` — `{ "id": "<call-ULID>", "body": { "reason": "..." } }`.
- `endCall` — `{ "id": "<call-ULID>" }`.

## mDNS + SDP hygiene

Chrome emits ICE candidates with `.local` mDNS hostnames for privacy (`XXXX.local`). Meta rejects the SDP with error `138008` when these are present. Nudgeway rewrites the SDP server-side before forwarding — you don't need to do anything client-side.

The `a=ice-options:trickle` line is also stripped for the same reason.

## Troubleshooting

- **Popup never appears** — the `/ws/inbox` socket dropped. Refresh; check `wss://` in DevTools Network.
- **Accept fails with Meta error `138008`** — SDP contained mDNS candidates or trickle-ICE options that weren't stripped. Check Nudgeway's version and the call adapter logs.
- **`getCallSession` returns `404`** — webhook ordering issue; the `connect` webhook hasn't landed yet. Wait a second and retry.
- **Audio one-way** — usually a browser mic-permission miss. Click the padlock in the address bar and grant mic access.
- **Call rings but auto-declines after a few seconds** — Meta's ring timeout. Answer faster, or check for a webhook processing lag.

## Related

- [Calls overview](/#/calls/overview) — full lifecycle + info-message dedup.
- [Outbound calls](/#/calls/outbound-call) — place a call from the operator side.
- [Recording + transcript](/#/calls/recording-transcript) — access after the call ends.
