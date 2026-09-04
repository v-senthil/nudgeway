# Calls troubleshooting

The common call failure modes and one-line fixes. Every provider invocation is captured in the [Provider calls log](/#/audit-telemetry/provider-calls); start there when the surface error isn't enough.

## Permission denied

- **`initiateCall` returns `502` with `no_permission` in the response detail** — the recipient hasn't granted call permission. Send a [`sendCallPermissionRequest`](/#/calls/call-permissions) and wait for them to tap Accept.
- **Call button greyed out** — the UI checked `getIntegrationCallPermission` and got `no_permission`. Use the **Request permission** affordance on the button.
- **Permanent permission suddenly `no_permission`** — the user revoked it in their WhatsApp settings, or blocked the business number. Nothing to do server-side.
- **`403 missing calls.manage`** on `initiateCall` — role doesn't grant call management.
- **`403 missing calls.read`** on `getIntegrationCallPermission` — role doesn't grant call reads.

## WebRTC failed

- **Meta error `138008`** on `answerCall` — SDP contained mDNS candidates or `a=ice-options:trickle`. Nudgeway strips these server-side; if you're still seeing 138008 the strip regressed. Check the call adapter logs + open an issue.
- **`getCallSession` returns `404`** — the `connect` webhook hasn't landed yet, or Meta didn't send one for this call (rare). Retry after a second; if the offer never appears, the call can't be answered in-browser.
- **Audio one-way** — browser mic permission missing. Click the padlock, grant mic. On macOS, System Settings → Privacy & Security → Microphone → Chrome/Safari.
- **`getUserMedia` throws `NotAllowedError`** — user dismissed the mic prompt. Refresh the tab and re-accept.
- **WebRTC works on localhost, fails in staging** — TLS required. WebRTC + mic access are blocked on plain HTTP outside localhost.
- **ICE never completes** — a corporate firewall is blocking UDP or TURN. WhatsApp calls need working UDP outbound; whitelist Meta's STUN/TURN or route through an ICE relay.

## Recording missing

- **Call ended but `recording_url` still empty** — the terminate webhook hasn't landed yet, or recording wasn't enabled. Check the integration's [Call settings](/#/integrations/call-settings) and the [Provider calls log](/#/audit-telemetry/provider-calls).
- **`getCallRecording` returns `404`** — call ULID is wrong, or the call belongs to another org.
- **`getCallRecording` returns `502`** — Meta's short-lived URL expired. The two-hop is idempotent; the retry worker will re-fetch, or you can re-trigger by re-firing the terminate webhook.
- **Recording is silent** — the recipient never joined WebRTC (dropped mid-negotiation). Duration will be near zero; nothing to recover.
- **`getCallTranscript` returns `409 not_available`** — transcription still running. Poll every 10-30s.

## SDP validation

- **Meta rejects the SDP with generic `bad_request`** — usually a media-line mismatch (offered audio, answered with video, etc.). Inspect the answer SDP in the DevTools console before it's POSTed.
- **Chrome mDNS candidates leaking to Meta** — should be impossible after the server-side strip; if you see them in the outbound provider log, file a bug with the raw SDP.

## Call state stuck

- **Info message stuck on `ringing`** — Meta didn't push the terminate webhook. `endCall` closes the row; the missing webhook will backfill later.
- **Duplicate info messages** — the `(call_id, status)` dedup key regressed. File a bug with the two conflicting rows.
- **Popup ghost — call already completed** — the `/ws/inbox` socket dropped and reconnected out of order. Refresh.

## Related

- [Calls overview](/#/calls/overview) — full lifecycle.
- [Call permissions](/#/calls/call-permissions) — check + request.
- [Recording + transcript](/#/calls/recording-transcript) — two-hop download.
