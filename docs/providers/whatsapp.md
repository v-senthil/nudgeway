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

## TODO — not yet supported

- Group messaging (`groups.md`).
- Calls (Phase 3): `calling.md`.
- Catalog + product / order webhooks (`catalogs/`, `webhooks/reference/messages/order.md`).
- Payments (`payments/`).
- Business Capability Update and other WABA-level webhooks (`webhooks/reference/*_update.md`).
- Media upload endpoint (`POST /<phone_number_id>/media`) — Phase 1 only downloads.
- NFM (Flows) inbound response typed decoding — currently preserved as raw.
- Ad referral (`referral`) — preserved in raw for now.
