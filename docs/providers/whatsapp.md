# WhatsApp Business Cloud API — adapter

Package: `internal/providers/whatsapp`.
Source of truth: `~/Documents/whatsapp_doc_tracker/docs/` (local mirror of Meta developer docs).

## Capabilities

| Capability | Supported | Notes |
|------------|-----------|-------|
| SendText | ✅ | `text` payload with optional link preview. |
| SendMedia | ✅ | image / video / audio / document / sticker; either `id` (uploaded) or `link` (URL). |
| SendTemplate | ✅ | Template + language + components. |
| ReceiveMessages | ✅ | Webhook parser handles text, media, location, contacts, interactive, button (template reply), reaction, system, and preserves unknown types. |
| Templates (mgmt) | ✅ | Create, list, get status via `message_templates`. |
| Groups | ❌ | Not modelled yet. |
| Calls | ❌ | Phase 3. |
| Flows | ✅ (send) | Interactive payloads pass through; NFM inbound responses land under `interactive.nfm_reply` — preserved as raw for now. |

## Provisioning

Operators can wire a WhatsApp phone number in one of two ways: the settings UI or the CLI. Both paths ultimately land the same rows: one in `integrations`, one envelope-encrypted blob in `integration_credentials`, and one in `business_endpoints` keyed by the WhatsApp `phone_number_id`.

### CLI (headless / bootstrap / CI)

```bash
nudgeway-cli integration create \
  --org-slug acme --provider whatsapp --name "Acme Support" \
  --phone-number-id 106540352242922 \
  --waba-id 102290129340398 \
  --access-token EAAJ... \
  --app-secret 3f5ab9... \
  --verify-token my-webhook-verify-token
```

The command is idempotent on `(org, provider, name)` — rerunning with the same name updates the ciphertext but keeps the same integration id (and therefore the same webhook URL). On success it prints:

```
integration created: id=01H..., webhook_url=/webhooks/whatsapp/01H...
```

Paste that path (with the app's public origin prepended) into the Meta app's Webhooks configuration, along with the same `--verify-token` value.

### Settings UI (`/settings/integrations`)

The form collects the same six fields:

| Field | Where it goes | Kind |
|-------|----------------|------|
| Name | `integrations.name` | non-secret |
| Phone number id | `integrations.config.phone_number_id` + `business_endpoints.external_id` | non-secret |
| WABA id | `integrations.config.waba_id` | non-secret |
| Access token | `integration_credentials.ciphertext[access_token]` | envelope-encrypted |
| App secret | `integration_credentials.ciphertext[app_secret]` | envelope-encrypted |
| Verify token | `integration_credentials.ciphertext[verify_token]` | envelope-encrypted |

The REST create endpoint (`POST /api/v1/integrations`) rejects secrets in the `config` object (validation error) — they must be sent in the `secrets` object so the service can envelope-encrypt them before writing.

### Health check

`POST /api/v1/integrations/{id}/test` resolves the adapter via the runtime `ProviderResolver` (wired in `cmd/server`) and calls `channel.Provider.HealthCheck`. The WhatsApp adapter reports the last-known outbound health flag rather than issuing a Graph ping — Meta rate-limits app-review pings and day-to-day sends are the truest signal. The service persists `Status` = `connected` on `OK`, else `degraded`, and stashes `{ok, message, checked_at}` on `Health` for the UI.

## Config

```go
whatsapp.Config{
    PhoneNumberID: "…",       // required for sends
    WABAID:        "…",       // required for template mgmt
    AccessToken:   "…",       // system/business integration token
    AppSecret:     "…",       // for X-Hub-Signature-256 verification
    GraphVersion:  "v20.0",   // default DefaultGraphVersion
    BaseURL:       "https://graph.facebook.com", // override for tests
    HTTPClient:    nil,       // defaults to 30s-timeout client
}
```

Credentials are envelope-encrypted at rest (`integration_credentials.ciphertext`, KEK in env) and NEVER logged.

## Canonical → Meta mapping (outbound)

| Canonical `MessageType` | Meta `type` | Notes |
|-------------------------|-------------|-------|
| `text` | `text` | `body` + optional `preview_url`. |
| `image` / `video` / `audio` / `document` / `sticker` | same | `id` or `link`; caption for image/video/document. |
| `template` | `template` | `name`, `language.code`, `components`. |
| `location` | `location` | `latitude`, `longitude`, optional `name`/`address`. |
| `reaction` | `reaction` | `message_id`, `emoji` (empty removes). |
| `interactive` | `interactive` | pass-through JSON; caller composes list/button/CTA/flow. |

`SendRequest.IdempotencyKey`, when present, is placed on the Meta payload as `biz_opaque_callback_data` — Meta echoes this on status webhooks, enabling idempotent send bookkeeping.

## Meta → canonical webhook mapping (inbound)

Only the `messages` field of the webhook envelope is consumed in Phase 1. Signature is verified via `VerifySignature(headers, rawBody, appSecret)` **before** parsing.

Inbound messages fan out as `events.MessageReceived` envelopes with `MessageReceivedPayload`. Status updates fan out as `MessageSent`/`Delivered`/`Read`/`Failed` with `MessageStatusPayload`.

| Meta `type` | Canonical `MessageType` | Payload |
|-------------|-------------------------|---------|
| `text` | `text` | `TextPayload` |
| `image` / `video` / `audio` / `document` / `sticker` | same | `MediaPayload` (with `media_id`, `mime_type`, `sha256`, …) |
| `location` | `location` | `LocationPayload` |
| `contacts` | `contacts` | `ContactsPayload` |
| `interactive.list_reply` | `interactive` | `InteractivePayload{ListReply}` |
| `interactive.button_reply` | `interactive` | `InteractivePayload{ButtonReply}` |
| `button` (template quick-reply) | `button` | `InteractivePayload{ButtonReply}` from `payload`/`text` |
| `reaction` | `reaction` | `ReactionPayload` |
| `system` | `system` | passthrough map |
| anything else | `unknown` | raw preserved in `MessageReceivedPayload.Raw` |

The parser also indexes the `contacts` array of the value block to attach `FromDisplayName` to the emitted event without a second scan.

### DTO surface for specialised inbound types

The `InboundService` also flattens the following canonical payloads into the persisted message's `Metadata` bag so the REST `Message` DTO (see `openapi.yaml → components.schemas.Message`) can render without a second lookup:

| Canonical `MessageType` | DTO field(s) | Notes |
|-------------------------|--------------|-------|
| `location` | `location: {latitude, longitude, name?, address?, url?}` | Frontend renders a pin + venue name + "Open in Maps" link. |
| `contacts` | `contacts: [{name, phones?, emails?}]` | Compact projection of the full vCard — the raw card is still available on the domain `Message.Metadata`. |
| `reaction` | `reaction: {emoji, message_id}` | `message_id` is the wamid of the reacted-to message; frontends overlay a badge on the referenced bubble. |
| `interactive` / `button` | `interactive: {kind, id?, title?, description?}` | One shape covers `button_reply`, `list_reply`, and template quick-reply `button`. |
| any reply (`context.id` set) | `reply_to_provider_message_id: string` | Set whenever the inbound envelope carries a `context.id`, regardless of message type. |

## Signature verification

Meta signs webhook bodies with `X-Hub-Signature-256: sha256=<hex hmac_sha256(body, app_secret)>`. The adapter computes the MAC over the raw request body (no re-serialisation) and compares in constant time with `hmac.Equal`.

## Rate limits

Meta throttles both sends and template management. The adapter classifies HTTP `429` and Meta error codes `4`, `80007`, `130429` as `ClassRateLimited` and applies exponential backoff (250 ms base, 5 s cap) with jitter. Callers on top of the port receive a wrapped `*APIError` — check `Retryable()`.

Full limits are documented in `whatsapp_doc_tracker/docs/messaging-limits.md`, `throughput.md`, and `pricing.md`.

## Error classification

| Class | HTTP / Meta signals | Caller action |
|-------|---------------------|---------------|
| `transient` | 5xx, network | retry with backoff |
| `rate_limited` | 429, codes 4/80007/130429 | back off longer |
| `auth` | 401/403, `OAuthException` | refresh token; mark integration degraded |
| `permanent` | other 4xx | do not retry; surface to UI |
| `unknown` | fallthrough | log + escalate |

## Media inbound flow

Meta webhooks for image/video/audio/document/sticker only carry a
`media_id` handle — the raw bytes must be fetched separately. The
adapter exposes `Provider.DownloadMedia(ctx, mediaID) (io.ReadCloser,
contentType, error)` which packages Meta's two-step pattern:

1. `GET https://graph.facebook.com/<version>/<media_id>` → JSON with a
   short-lived signed `url` field, `mime_type`, `sha256`, `file_size`.
2. `GET <url>` with the Bearer access token → raw bytes streamed back.

The `InboundService` drives this via the provider-agnostic
`AttachmentDownloader` port (`internal/application/message/deps.go`),
so no provider package is imported from the application layer. On each
media envelope the service:

1. Calls `Downloader.Download(ctx, "whatsapp", integrationID, mediaID)`.
2. Streams the bytes into `attachments.Store.Put(ctx, contentType, r)` —
   the dev implementation is a local filesystem store rooted at
   `attachments.root` in `config/local.yaml`, content-addressed by
   SHA-256 with a `.contenttype` sidecar file.
3. Stamps `attachment_key`, `content_type`, `file_size` on
   `message.metadata` so the REST DTO surfaces `media_url =
   /api/v1/media/<key>` and `content_type` on `Message`.
4. Download failures are WARN-logged and swallowed — the message row
   still persists so the thread never blocks on a transient Meta CDN
   outage; the browser renders an "Attachment unavailable" fallback.

The served endpoint (`GET /api/v1/media/{key}`, `HEAD` for prefetch)
is auth-gated so downloaded media stays inside the tenant boundary.
Cache headers are `private, max-age=86400` since content-addressed keys
never change semantically.

For the fuller Meta metadata surface (`SHA256`, `FileSize`, …), the
adapter also exposes `Provider.DownloadMediaMetadata(...)` which wraps
the same two calls into a `*Media` value.

## Media outbound flow

Operator picks a file in the composer → `POST /api/v1/attachments` (multipart)
uploads it into the content-addressed attachments store → server returns
`{attachment_id, media_url}` → composer calls `POST /api/v1/messages` with
`type=image|video|audio|document` and `media: {url: <media_url>, caption?}` →
`SendService.RequestSend` persists a `queued` row and enqueues on `message.send`
→ the send worker calls the WhatsApp adapter → `canonicalSendToMeta` turns
the canonical `MediaPayload.URL` into `{type:"image", image:{link:"<url>"}}`
(or the corresponding video/audio/document/sticker shape) → Meta fetches the
URL from `PublicBaseURL` and delivers to the recipient.

The upload endpoint enforces a 16 MiB cap. Larger blobs will require the
Meta resumable upload API (`POST /<phone_number_id>/uploads` in the docs) —
still TODO in Phase 1. Content-type sniffing falls back to
`http.DetectContentType` when the multipart part omits the header. The
returned `media_url` is prefixed with `PublicBaseURL` so Meta's fetcher can
reach it; a same-origin SPA also works when `PublicBaseURL` is empty.

## Mark as read

The `channel.Provider.MarkAsRead` port is implemented by
`Provider.MarkAsRead(ctx, providerMessageID)` (`provider.go`). It POSTs to
`/{phone_number_id}/messages` with:

```json
{
  "messaging_product": "whatsapp",
  "status": "read",
  "message_id": "wamid...."
}
```

Meta shows the customer's client a blue double-tick, and marks earlier
messages in the same conversation as read as well (that's Meta's
behaviour, not ours). Reference:
`~/Documents/whatsapp_doc_tracker/docs/messages/mark-message-as-read.md`.

Constraints:

- Must be called within 30 days of the inbound receipt; older wamids
  return Meta error 131009.
- Meta does **not** send us a status callback for a business-side read
  (the "read" event only exists on the customer's side). `ReadService`
  therefore stamps `messages.read_at` locally.
- The call is idempotent on our side: `ReadService.MarkRead` skips
  outbound rows, rows without a `provider_message_id`, and rows already
  stamped `read_at`.

Wire path: `POST /api/v1/messages/{id}/read` or the batch
`POST /api/v1/conversations/{id}/read` → `ReadService.MarkRead` /
`MarkConversationRead` → `provider.MarkAsRead` → `client.markAsRead`.

## Media upload (Meta-native media_id)

Meta's `POST /{phone_number_id}/media` (see `whatsapp_doc_tracker/docs/messages/media/upload.md`) returns a first-class `media_id` handle that is preferred over `link` URLs — Meta already has the bytes, so subsequent sends are one round-trip instead of two.

Wire path on `POST /api/v1/attachments`:

1. Handler streams the multipart part into `attachments.Store.Put` — HBase writes `d:content` (bytes) + `m:content_type` (MIME) + `m:size`. Row key is the SHA-256 hex of the bytes (content-addressed dedupe).
2. Handler then calls the wire-up `MediaUploader`. `cmd/server/main.go`'s `metaMediaUploader` picks the first WhatsApp integration for the org, opens the stored bytes back from the store, and hits `provider.UploadMedia(ctx, contentType, filename, r)`.
3. `client.uploadMedia` builds the multipart body (`file`, `messaging_product=whatsapp`, `type=<mime>`) and POSTs to `/{phone_number_id}/media` with the tenant's access token. Meta's response `{id: "<media_id>"}` is stashed back on the HBase row under `m:media_id_<provider>_<integration>`.
4. The 201 response returns both `media_url` and `media_id` (plus `provider`, `size`, `content_type`, `filename`). The composer prefers `media_id`.

Failure is best-effort: if Meta rejects the upload the local blob still exists and the composer falls back to `media_url` (a same-origin `/api/v1/media/<key>` link). Boot logs a WARN when `MediaUploader` is nil (no WhatsApp integration wired), and per-upload failures log at WARN with the request-id so operators can correlate.

## BSUID (business-scoped user IDs)

Meta is migrating from phone-number-based `wa_id` to business-scoped user IDs (BSUID) — format `<CC>.<alnum-up-to-128>`, e.g. `IN.10173928811470384`. See `~/Documents/whatsapp_doc_tracker/docs/business-scoped-user-ids.md` for the full rollout timeline; in the target end state the contacts block will *only* carry `user_id` and `wa_id` will be omitted.

### Inbound

The mapper reads three shapes at once:
- `contacts[].user_id`, `contacts[].parent_user_id`, `contacts[].profile.username`
- `messages[].from_user_id`, `messages[].from_parent_user_id`
- `statuses[].recipient_user_id`

`webhook.go` indexes the contacts block by both `wa_id` and `user_id` so `MessageReceivedPayload` carries `FromUserID`, `FromParentUserID`, `FromUsername` regardless of which key the message uses.

`InboundService.handleReceived` upserts a `bsuid` `ContactIdentity` bound to the same Contact and promotes it to the Contact's `primary_identity_id`. The phone-based identity is retained (customers may still send from either shape during the rollout window).

### Outbound

`SendService.resolveRecipient` iterates the target Contact's identities. Order today: `phone` / `whatsapp` first, `bsuid` as fallback — Meta's *portfolio-side send-by-BSUID* rollout is partial, and a phone number is universally accepted right now. Once the rollout completes the order will flip to BSUID-first. Meta rejects unknown BSUIDs with `Invalid parameter` (code 100); that failure is surfaced as a canonical `MessageFailed`.

### Status callbacks

`MessageStatusPayload.RecipientUserID` is populated from `statuses[].recipient_user_id`. The rest of the pipeline is BSUID-agnostic — it looks the row up by `provider_message_id`, which is what makes `SetProviderMessageID` (stamped at send-success time) load-bearing for the delivered / read tick advance.

## TODO — not yet supported

- Group messaging (`groups.md`).
- Calls (Phase 3): `calling.md`.
- Catalog + product / order webhooks (`catalogs/`, `webhooks/reference/messages/order.md`).
- Payments (`payments/`).
- Business Capability Update and other WABA-level webhooks (`webhooks/reference/*_update.md`).
- Meta resumable upload API for attachments > 16 MiB (`POST /<phone_number_id>/uploads`).
- BSUID-first send (once Meta portfolio-side send accepts all BSUIDs universally).
- NFM (Flows) inbound response typed decoding — currently preserved as raw.
- Ad referral (`referral`) — preserved in raw for now.

## Observability — Meta API execution logs

Every outbound HTTP call the WhatsApp adapter makes is recorded to the
`provider_calls` MySQL table for operator debugging. See
[`docs/domain/provider_call.md`](../domain/provider_call.md) for the entity
and [`docs/flows/provider-call-recording.md`](../flows/provider-call-recording.md)
for the sequence.

Recorded operations (from `internal/providers/whatsapp/`):

| Operation | Callsite | Records request body | Records response body |
|-----------|----------|----------------------|-----------------------|
| `send_message` | `client.sendMessage` | yes | yes |
| `mark_as_read` | `client.markAsRead` | yes | yes |
| `get_media_url` | `client.getMediaURL` | no (GET) | yes |
| `download_media` | `client.downloadMedia` | no (GET) | **no** — raw bytes are the media itself |
| `list_templates` | `client.listTemplates` | no (GET) | yes |
| `create_template` | `client.createTemplate` | yes | yes |
| `get_template_status` | `client.getTemplateStatus` | no (GET) | yes |
| `upload_media` | `client.uploadMedia` | synthetic `{filename, content_type, size}` — never the raw multipart bytes | yes |

Every entry carries `status_code`, `latency_ms`, `error_class`,
`error_message`, and Meta's `fbtrace_id` (from the `x-fb-trace-id` header
or the error envelope's `fbtrace_id` field). On retries, each attempt
emits its own entry so the retry history is visible newest-first.

**Never stored:** the `Authorization: Bearer <token>` header. The tracer
interface at `internal/providers/whatsapp/tracer.go` has no headers field
on `TraceEvent`; adding one would require growing `Entry.Redact()` first.

**Read surface:** `GET /api/v1/provider-calls` behind
`integrations.manage`. Frontend viewer: `/settings/provider-calls`.
