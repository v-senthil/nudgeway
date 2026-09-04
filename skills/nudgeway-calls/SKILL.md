---
name: nudgeway-calls
description: WhatsApp Business calls — inbound popup, in-browser WebRTC accept, call permissions, recording + transcript persistence. Every call state transition becomes an in-thread info message.
trigger: User asks about voice calls, accepting an inbound WhatsApp call, requesting call permission, or looking at call recordings / transcripts.
---

# Nudgeway calls skill

## Overview

WhatsApp Business Calling is a full first-class channel in Nudgeway. Inbound calls surface as a bottom-right popup with Accept / Decline; accepting negotiates WebRTC in the browser and connects audio. Every call is mirrored into the conversation thread as a series of info messages (`ringing → answered → completed`), with the terminal transition click-through to the call detail (recording + transcript).

## Surface (partially in openapi.yaml — call REST directly for admin operations)

```
GET  /api/v1/calls                          — list calls for the org
GET  /api/v1/calls/{id}                     — one call with full transitions
POST /api/v1/calls                          — initiate an outbound call
POST /api/v1/calls/permission-request       — send a call_permission_request message
GET  /api/v1/integrations/{id}/call-permission?to=<E164>
                                             — check current permission state
```

The `postCallPermissionRequested` audit action fires whenever a permission-request message is sent.

## Patterns

### Check call permission before initiating

```bash
GET /api/v1/integrations/{id}/call-permission?to=918197002143
→ { "status": "permanent" | "temporary" | "none", "expiration_time": 1730000000 }
```

- `permanent`: initiate freely.
- `temporary`: initiate before `expiration_time`.
- `none`: send a `call_permission_request` message first.

### Request call permission

```bash
POST /api/v1/calls/permission-request
{
  "integration_id": "01M1...",
  "to": "918197002143",
  "message": "May we call you to discuss your recent order?"
}
```

The customer sees an interactive prompt in WhatsApp; their `accept` / `reject` reply is rendered inline as a `call_permission_reply` bubble.

### Read the recording + transcript

Recording + transcript are captured via a Meta webhook after the call ends, downloaded to HBase, and rendered on the call detail page. Access:

```
GET /api/v1/calls/{id}                    — includes recording_url + transcript_ref
GET /api/v1/media/{content_hash}          — stream the media bytes
```

## Gotchas

- **WebRTC requires HTTPS**: browsers block mic access on plain HTTP outside localhost. `make dev` on localhost works; production requires TLS.
- **mDNS candidate stripping**: Chrome's mDNS candidates and `a=ice-options:trickle` are stripped from the SDP answer before it hits Meta (Meta error 138008 otherwise).
- **Info-message dedup**: each `(call_id, status)` tuple produces exactly one info message. Duplicate webhook deliveries are absorbed.
- **Recording proxy**: Meta returns a `recording_media_id` (media asset) — Nudgeway does a two-hop download (media_id → URL → bytes) before persisting to HBase.

## Related skills

- [`nudgeway-inbox`](../nudgeway-inbox/SKILL.md) — call transitions render as info messages inside the same thread.
- [`nudgeway-integrations`](../nudgeway-integrations/SKILL.md) — call settings (call hours, callback permission, call icon visibility) live on the integration settings drawer.
