---
name: nudgeway-integrations
description: Provider integrations — create a WhatsApp integration, run a connection test, push the webhook to Meta, delete an integration. Credentials are envelope-encrypted; secrets never leave the backend.
trigger: User asks about connecting WhatsApp, adding a phone number, testing an integration, or setting the Meta webhook.
---

# Nudgeway integrations skill

## Overview

An **Integration** is one connection to a provider (WhatsApp today; more adapters land as they ship). For WhatsApp, one integration = one Meta Phone Number ID. Config (non-secret) and Secrets (encrypted with per-org KEK/DEK) are stored separately.

## MCP tools

| operationId | Purpose |
|---|---|
| `listIntegrations` | List all integrations owned by the caller's org. Also returns the per-integration webhook URL. |
| `getIntegration` | Fetch one integration by ULID. |
| `createIntegration` | Create a new integration. Body: `{type, provider, name, config, secrets}`. |
| `testIntegration` | Health-check the integration by calling Meta. Flips `status` to `connected` or `degraded`. |
| `deleteIntegration` | Soft-disconnect. Row + encrypted credentials remain for audit; status flips to `disconnected`. |

Note: **Set-webhook** (push callback URL to Meta) lives on the settings-drawer surface and is not currently in the OpenAPI spec — call `POST /api/v1/integrations/{id}/webhook` directly with `{"public_url":"https://..."}`.

## REST equivalents

```
GET    /api/v1/integrations
POST   /api/v1/integrations
GET    /api/v1/integrations/{id}
POST   /api/v1/integrations/{id}/test
DELETE /api/v1/integrations/{id}
POST   /api/v1/integrations/{id}/webhook     ← push callback to Meta
GET    /api/v1/tools/ngrok-tunnel            ← best-effort ngrok probe
```

## Patterns

### Create a WhatsApp integration

```json
{
  "tool": "createIntegration",
  "arguments": {
    "body": {
      "type": "channel",
      "provider": "whatsapp",
      "name": "Acme India",
      "config": {
        "phone_number_id": "1017392881147...",
        "waba_id": "1236225006237445"
      },
      "secrets": {
        "access_token": "EAAECm...",
        "app_secret": "89ab...",
        "verify_token": "a0ea12cb..."
      }
    }
  }
}
```

### Push webhook override to Meta

```bash
POST /api/v1/integrations/{id}/webhook
{"public_url": "https://a1b2.ngrok-free.app"}
```

The backend appends `/webhooks/whatsapp/{id}` and reuses the stored `verify_token` — you never send those fields.

### Health check

```json
{ "tool": "testIntegration", "arguments": { "id": "<integration-ULID>" } }
```

## Gotchas

- **Secrets in config bag**: rejected with 422. Always put `access_token` / `app_secret` / `verify_token` under `secrets`, not `config`.
- **Provider registry**: the provider key must be registered at process boot (`internal/providers/registry.go`). Unknown providers return 422 at create time.
- **Business endpoint upsert**: creating a WhatsApp integration also upserts a `BusinessEndpoint` row keyed by `phone_number_id`. Creating two integrations with the same phone number will collide.
- **Delete is soft**: to fully remove, drop the row via the CLI (`nudgeway-cli integration delete ...`, if implemented) or SQL.

## Related skills

- [`nudgeway-inbox`](../nudgeway-inbox/SKILL.md) — messages only flow after `testIntegration` reports `connected`.
- [`nudgeway-analytics`](../nudgeway-analytics/SKILL.md) — provider-call telemetry captures every Meta round-trip for each integration.
