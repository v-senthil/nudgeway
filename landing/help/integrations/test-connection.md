# Test the connection

`testIntegration` resolves the provider adapter, runs its `HealthCheck`, updates the integration's `Status` + `Health` columns, and returns the outcome. Use it after creating an integration, after rotating credentials, or any time an operator wants a fresh probe.

## How to use

1. Settings → Integrations → the row you want to test → **Test connection**.
2. Watch the status pill flip.

## Status transitions

- `pending` → `connected` — the initial state after `POST /api/v1/integrations` becomes `connected` on the first green health check.
- `connected` → `degraded` — a health check that returns a Meta error (network timeout, 5xx, rate limit) flips to `degraded`. Workers may still try; the provider-call log has the reason.
- `degraded` → `connected` — a subsequent green check restores it.
- `*` → `auth_failed` — Meta returned `401` / `190` (invalid access token). The token needs rotating.
- `*` → `rate_limited` — Meta throttled the WABA. Wait or contact Meta.

## API

```
POST /api/v1/integrations/{id}/test
```

No request body. Requires CSRF + `integrations.manage`.

Response `200`:

```json
{
  "ok": true,
  "message": "phone_number_id=1017…: quality_rating=GREEN, verified_name=\"Acme Support\"",
  "checked_at": "2026-09-05T09:20:14Z"
}
```

Failure example:

```json
{
  "ok": false,
  "message": "meta 401: (#190) The access token has expired",
  "checked_at": "2026-09-05T09:20:14Z"
}
```

Additional statuses: `403` (CSRF or permission), `404` (integration not found).

## MCP

| operationId | Purpose |
|---|---|
| `testIntegration` | Health-check the integration by calling Meta. Flips `status` to `connected` or `degraded`. |
| `getIntegration` | Read the current status + `health` map without probing. |

## What the health check does under the hood

For WhatsApp, `HealthCheck` calls `GET /<phone_number_id>?fields=verified_name,quality_rating,status` and records the response in `Integration.health`. The full HTTP round-trip is written to `provider_calls` under `operation=health_check` — inspect it via [Meta API execution log](/#/audit-telemetry/provider-calls).

## Troubleshooting

- **`degraded`, `meta 400` (invalid parameter)** — likely a bad `phone_number_id` in `config`. Re-verify from Meta's API Setup page.
- **`auth_failed`** — access token expired or revoked. Rotate via SQL / CLI (there is no re-key REST endpoint today).
- **`ok: false, "network refused"`** — outbound HTTP is blocked. Check the SSRF blocklist (`internal/infrastructure/http/client.go`) if you are running against a mock; production hits Meta at `graph.facebook.com`.

## Related

- [Integrations overview](/#/integrations/overview)
- [Connect a WhatsApp integration](/#/integrations/connect-whatsapp)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
