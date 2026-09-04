# Meta API execution log

The operator-facing execution log of every outbound HTTP call the provider adapters made to a third-party API. One row per round-trip — request URL, method, status, latency, base64-encoded request + response bodies (redacted, truncated at 64 KiB), and error class if the request failed.

This is the **canonical debugging surface** for "why did that message fail?" or "what did Meta actually return?".

## Row shape

| Field | Notes |
|---|---|
| `id` | Auto-increment BIGINT. |
| `org_id` | Tenant ULID. |
| `integration_id` | Owning integration. Absent when a very-early failure prevented loading the integration row. |
| `provider` | Registry key of the calling adapter (e.g. `whatsapp`). |
| `operation` | Adapter operation name — see below. |
| `direction` | `outbound` (only value written today). |
| `method` | HTTP verb. |
| `url` | Fully-qualified request URL. Never contains secrets. |
| `status_code` | HTTP status. `0` when the request never completed (network error). |
| `latency_ms` | Wall-clock duration of the request. |
| `request_body` / `response_body` | Base64-encoded, truncated at 64 KiB. Empty for GETs and for `download_media`. |
| `request_body_text` / `response_body_text` | Convenience decoded views (same 64 KiB cap). |
| `occurred_at` | RFC 3339. |

## Operations (per WhatsApp adapter)

| `operation` | What it maps to on Meta |
|---|---|
| `send_message` | `POST /<phone_number_id>/messages` |
| `mark_as_read` | `POST /<phone_number_id>/messages` with `status: "read"` |
| `get_media_url` | `GET /<media_id>` |
| `download_media` | `GET <media_url>` (Meta's CDN). `response_body` empty — blob is streamed to disk. |
| `upload_media` | `POST /<phone_number_id>/media` |
| `list_templates` | `GET /<waba_id>/message_templates` |
| `create_template` | `POST /<waba_id>/message_templates` |
| `get_template_status` | `GET /<template_id>` |
| `list_groups` | `GET /<phone_number_id>/groups` |
| `get_group` | `GET /<group_id>?fields=…` |
| `health_check` | `GET /<phone_number_id>?fields=…` |

New operations land in the adapter with matching tags; grep `internal/providers/whatsapp/` for `TraceEvent(...)`.

## Redaction

The middleware redacts sensitive keys at any JSON depth before persisting the request body:

- `password`
- `access_token`
- `app_secret`
- `verify_token`
- `secrets`
- `plaintext`
- `token`
- `secret`

Each is replaced with the string `"[redacted]"`. Non-JSON bodies pass through untouched (still capped at 64 KiB). Media / binary responses (content-type outside JSON / text / form) store `null` for `response_body` and only record the byte size.

## How to use

1. Settings → Audit → **Meta API execution log** tab.
2. Filter combinations:
   - `integration_id` — scope to one WhatsApp integration.
   - `operation` — scope to `send_message`, `list_groups`, ...
   - `status_min` / `status_max` — bound the HTTP status (e.g. `400..599` = failures only).
   - `since` / `until` — RFC 3339 range.
3. Click a row to expand the request + response body panels.

## API

```
GET /api/v1/provider-calls
  ?integration_id=01J…
  &operation=send_message
  &status_min=400
  &status_max=599
  &since=2026-09-05T00:00:00Z
  &until=2026-09-05T23:59:59Z
  &cursor=eyJ…
  &limit=50
```

Response `200`:

```json
{
  "items": [
    {
      "id": 91834,
      "org_id": "01J…",
      "integration_id": "01J…",
      "provider": "whatsapp",
      "operation": "send_message",
      "direction": "outbound",
      "method": "POST",
      "url": "https://graph.facebook.com/v20.0/1017…/messages",
      "status_code": 400,
      "latency_ms": 214,
      "request_body_text": "{\"messaging_product\":\"whatsapp\",...}",
      "response_body_text": "{\"error\":{\"code\":131047,\"message\":\"Re-engagement message\"}}",
      "occurred_at": "2026-09-05T09:31:14Z"
    }
  ],
  "next_cursor": "eyJ…"
}
```

Requires `integrations.manage`. Additional statuses: `400` invalid cursor / filter.

## MCP

| operationId | Purpose |
|---|---|
| `listProviderCalls` | List Meta / provider execution log entries with filters + cursor pagination. |

## Troubleshooting

- **Row is missing** — a `download_media` binary response is captured with an empty `response_body` (correct). If a real send is missing, the tracer isn't wired for that operation; grep the adapter for `TraceEvent(...)`.
- **Body is `[redacted]`** — the redactor found one of the sensitive keys. Adjust the payload path or accept — that's the point.
- **`status_code: 0`** — the request never completed. Check the corresponding slog line for the transport error class.
- **Huge response truncated** — 64 KiB cap. Re-run the request manually with curl if you need the full body.

## Related

- [Audit & Meta telemetry overview](/#/audit-telemetry/overview)
- [Audit log](/#/audit-telemetry/audit-log)
- [Integrations overview](/#/integrations/overview)
- Source of truth: `internal/api/openapi/openapi.yaml` (`ProviderCall` schema), `docs/api-token-usage.md` (analogous per-token log).
