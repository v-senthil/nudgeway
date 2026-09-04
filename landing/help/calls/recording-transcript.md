# Recording + transcript

When a call ends, Meta fires a webhook carrying a `recording_media_id` (and, if transcription was requested, a `transcription_ref`). Nudgeway does a two-hop download (media_id → short-lived URL → bytes), persists to HBase under a content-addressed key, and stamps the call row. The call detail page then renders an audio player + a transcript view.

## The two-hop download

Meta media IDs are short-lived and require a Bearer token to fetch. To avoid the browser ever seeing that token, Nudgeway:

1. Receives `recording_media_id` on the `calls` webhook `terminate` event.
2. Hits Meta's `/graph.facebook.com/{media_id}` endpoint with the integration's access token → gets a short-lived pre-signed URL.
3. Downloads the audio bytes from that URL.
4. Persists to HBase under `sha256:<content-hash>`.
5. Stamps `calls.recording_url` on the row.

## How to use

- **UI**: click the terminal info message in the thread (`completed` / `missed` / `failed`) — it deep-links to `/calls/{id}` where the audio player + transcript live.
- **API**: `getCallRecording` streams the audio bytes; `getCallTranscript` streams the raw transcript JSON.

## API

### Enable recording + transcription on outbound

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/calls' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "01M1…",
    "to": "918197002143",
    "recording": { "enabled": true },
    "transcription": { "enabled": true, "language": "en" }
  }'
```

For inbound calls, recording + transcription defaults come from [Call settings](/#/integrations/call-settings) on the integration.

### Stream the recording

**operationId**: `getCallRecording`

```
GET /api/v1/calls/{id}/recording
```

Proxies the audio from the provider so the browser never sees the Meta short-lived URL.

```bash
curl -sS 'http://127.0.0.1:8080/api/v1/calls/01M1…/recording' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -o recording.ogg
```

Content-type is one of `audio/ogg`, `audio/mp4`, `audio/wav`, or `application/octet-stream` depending on what Meta returned.

### Fetch the transcript

**operationId**: `getCallTranscript`

```
GET /api/v1/calls/{id}/transcript
```

Returns the raw provider transcript JSON (opaque shape — pass through). Returns `409 not_available` when the `transcription_ref` isn't stamped yet, so the UI can render a "not available yet" affordance.

## MCP

- `getCall` — `{ "id": "<call-ULID>" }` to read the full call including `recording_url` and `transcription_ref`.
- `getCallRecording` — streams bytes; use with a Bearer token and pipe to a file.
- `getCallTranscript` — returns transcript JSON.

## HBase storage

Media persists in HBase under the `media` namespace, keyed by SHA-256 content hash. Deduplication is content-based — the same audio blob (e.g. the same voicemail replayed) stores once. The generic `GET /api/v1/media/{content_hash}` endpoint streams any media bytes (recording, attachment, transcript-adjacent).

## Troubleshooting

- **`409 transcript not available yet`** — Meta hasn't finished transcription. Polling every 10-30s usually resolves within a minute of call end.
- **`404 call not found`** — wrong ULID, or the call belongs to another org.
- **`424 integration missing`** — the integration was deleted; the two-hop can't run without the access token.
- **`502 provider error`** on recording fetch — Meta's short-lived URL expired between webhook and download. Re-fire the webhook (or wait for our retry) — the two-hop is idempotent.
- **Recording plays as static** — Meta occasionally returns `application/octet-stream` for an OGG blob. Save the bytes and inspect with `file`; `ffplay` reads it correctly regardless.

## Related

- [Calls overview](/#/calls/overview) — lifecycle + info-message dedup.
- [Inbound calls](/#/calls/inbound-call) — accept + WebRTC.
- [Outbound calls](/#/calls/outbound-call) — set `recording.enabled` at initiate.
