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
fullwa-cli integration create \
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
