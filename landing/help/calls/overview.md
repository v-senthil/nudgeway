# Calls overview

WhatsApp Business Calling is a first-class channel in Nudgeway. Inbound calls surface as a bottom-right popup with Accept / Decline; accepting negotiates WebRTC in the browser so the operator answers the call without leaving the tab. Every call state transition is mirrored into the conversation thread as an info message (`ringing → answered → completed`), with the terminal transition click-through to the call detail page (recording + transcript).

## Domain rules

- One call belongs to one integration, one contact, and one conversation. `direction` is either `inbound` or `outbound`.
- Call status enum: `queued`, `ringing`, `answered`, `in_progress`, `completed`, `missed`, `failed`, `declined`, `no_answer`.
- Each `(call_id, status)` tuple produces exactly one info message in the thread — duplicate webhook deliveries are absorbed.
- Recording + transcript are captured via a Meta webhook after the call ends, downloaded to HBase, and rendered on the call detail page.

## WebRTC in the browser

Accepting an inbound call:

1. Client fetches the SDP offer via `getCallSession`.
2. Client builds an `RTCPeerConnection`, generates an answer.
3. Answer is POSTed to `answerCall` with `body.sdp`.
4. Nudgeway strips Chrome's mDNS candidates and any `a=ice-options:trickle` line before forwarding to Meta (avoids error `138008`).
5. Meta and the browser exchange ICE; audio starts flowing.

WebRTC requires HTTPS. `make dev` on localhost works (browser exception); production requires TLS.

## Permissions

Before you can call a WhatsApp user, they must have granted you call permission — either `permanent` or `temporary` (an unexpired grant). Otherwise you have to send an interactive `call_permission_request` message and wait for the user to tap Accept. See [Call permissions](/#/calls/call-permissions).

## What ships in this module

| Feature | Page |
|---|---|
| Accept an inbound call | [Inbound calls](/#/calls/inbound-call) |
| Place an outbound call | [Outbound calls](/#/calls/outbound-call) |
| Check + request call permission | [Call permissions](/#/calls/call-permissions) |
| Recording + transcript | [Recording + transcript](/#/calls/recording-transcript) |
| Common failure modes | [Troubleshooting](/#/calls/troubleshooting) |

## Related

- [Inbox overview](/#/inbox/overview) — call transitions render inline in the thread.
- [Call settings](/#/integrations/call-settings) — per-integration call hours, callback permission, call icon visibility.
- [Meta Analytics tab](/#/analytics/meta-analytics-tab) — call analytics (`getMetaCallAnalytics`).
