# Send media (image / video / document)

Media sends are a two-step flow: upload the blob to Nudgeway's content-addressed store, then send a message referencing the returned `media_url`. The frontend hides the two steps behind the paperclip button, but the API exposes both.

## How to use

1. Click the paperclip in the composer.
2. Pick a file (image, video, audio, document, sticker). Max **16 MiB** — larger blobs need the resumable upload API (not yet supported).
3. Optionally add a caption.
4. Hit **Send**. The client uploads via `postAttachmentsUpload`, then calls `postMessagesSend` with the returned `media_url`.

The bubble renders WhatsApp-native — image / video thumbnails inline, documents as a filename + download chip.

## API

**Step 1 — upload.** `operationId`: `postAttachmentsUpload`

```
POST /api/v1/attachments   (multipart/form-data, one `file` field)
```

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/attachments' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -F 'file=@catalogue.pdf'
```

Response (`AttachmentUploadResponse`):

```json
{
  "attachment_id": "sha256:…",
  "media_url": "https://your-tunnel.trycloudflare.com/api/v1/media/sha256:…",
  "size": 812345,
  "content_type": "application/pdf",
  "filename": "catalogue.pdf"
}
```

**Step 2 — send.** `operationId`: `postMessagesSend`

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/messages' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "conversation_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
    "type": "document",
    "media": {
      "url": "https://your-tunnel.trycloudflare.com/api/v1/media/sha256:…",
      "mime_type": "application/pdf",
      "caption": "Latest catalogue",
      "file_name": "catalogue.pdf"
    },
    "idempotency_key": "doc-01M1…"
  }'
```

The valid `type` values for media are `image`, `video`, `audio`, `document`, `sticker`.

Prefer `media.url` (Nudgeway-hosted, content-addressed) over `media.media_id` (Meta-hosted). Meta media IDs are short-lived and can only be reused inside the WABA that uploaded them.

## MCP

Two tool calls:

```json
{ "tool": "postAttachmentsUpload", "arguments": { "body": <multipart> } }
{ "tool": "postMessagesSend", "arguments": { "body": {
    "conversation_id": "<conv-ULID>",
    "type": "image",
    "media": { "url": "<media_url from step 1>", "caption": "…" },
    "idempotency_key": "…"
}}}
```

## Troubleshooting

- **`413 Payload Too Large`** — file exceeds 16 MiB. Compress or split; resumable uploads land in a later phase.
- **Meta returns `media_not_reachable`** — Meta couldn't fetch `media_url`. Your public tunnel is down; check `cloudflared` / `ngrok`.
- **Wrong MIME type on Meta's side** — pass an explicit `media.mime_type` on the send. Meta trusts the header more than the URL extension.
- **Video won't play on the recipient's phone** — Meta only accepts H.264/AAC MP4 for `video`. Transcode with `ffmpeg -i in.mov -c:v libx264 -c:a aac out.mp4`.
- **Upload succeeds but send returns `424`** — the conversation's integration was deleted between the two calls. Re-check via [Test the connection](/#/integrations/test-connection).

## Related

- [Send a text message](/#/inbox/send-text) — the simpler path.
- [Send a template message](/#/inbox/send-template) — media in template headers.
- [Troubleshooting](/#/inbox/troubleshooting) — 24h window, WebSocket, attachment failures.
