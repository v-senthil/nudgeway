# Connect a WhatsApp integration

Settings → Integrations → **Connect WhatsApp**. The six-field form covers everything the WhatsApp Business Cloud adapter needs; secrets are envelope-encrypted before the row hits MySQL.

## The six fields

| Field | Where it comes from in Meta |
|---|---|
| **Name** | Any label. Displayed in the integration list. Example: "Acme India". |
| **Phone Number ID** | developer.facebook.com → your App → **WhatsApp → API Setup** → the number under "From". Copy the numeric id, not the display number. |
| **WABA ID** | Same **API Setup** page → the "WhatsApp Business Account ID" panel at the top. |
| **Access Token** | Business Settings → **Users → System Users** → your system user → **Generate new token**. Scopes: `whatsapp_business_messaging` + `whatsapp_business_management`. Prefer a permanent System User token; temporary 24-hour tokens will break the integration when they expire. |
| **App Secret** | developer.facebook.com → your App → **Settings → Basic** → **App Secret → Show**. Used to HMAC-verify incoming webhook bodies. |
| **Verify Token** | Any string you choose (e.g. `openssl rand -hex 16`). Save the same value in the form and in Meta's webhook Callback URL panel — Meta echoes it back during the subscription handshake. |

## How to use

1. Copy the six values from Meta.
2. Settings → Integrations → **Connect WhatsApp**.
3. Paste, save. The response returns the created `Integration` (secrets stripped) plus a `webhook_url` you'll paste into Meta.
4. [Test the connection](/#/integrations/test-connection).
5. [Push the webhook to Meta](/#/integrations/webhook-setup).

## API

```
POST /api/v1/integrations
Content-Type: application/json

{
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
```

Response `201`:

```json
{
  "id": "01J…",
  "org_id": "01J…",
  "type": "channel",
  "provider": "whatsapp",
  "name": "Acme India",
  "status": "connected",
  "config": { "phone_number_id": "…", "waba_id": "…" },
  "capabilities": { "text": true, "media": true, "template": true, "groups": true },
  "webhook_url": "https://app.example.com/webhooks/whatsapp/01J…",
  "created_at": "2026-09-05T09:14:00Z",
  "updated_at": "2026-09-05T09:14:00Z"
}
```

Requires CSRF + `integrations.manage`. Additional statuses: `400` bad request, `422` validation failure (unknown provider, missing config keys, secrets accidentally passed under `config`).

## MCP

| operationId | Purpose |
|---|---|
| `createIntegration` | Create a new integration. Body: `{type, provider, name, config, secrets}`. |
| `listIntegrations` | List every integration; also returns the per-integration `webhook_url`. |
| `getIntegration` | Fetch one by ULID. Secrets are stripped. |

## Gotchas

- **Secrets under `config` are rejected with 422**. Always put `access_token` / `app_secret` / `verify_token` under `secrets`.
- **`phone_number_id` collision** — creating a WhatsApp integration also upserts a `BusinessEndpoint` row keyed by phone_number_id. Two integrations with the same phone number collide.
- **Temporary tokens expire**. Use a System User permanent token, not a 24-hour test token.

## Related

- [Integrations overview](/#/integrations/overview)
- [Test the connection](/#/integrations/test-connection)
- [Push webhook to Meta](/#/integrations/webhook-setup)
- [First run](/#/getting-started/first-run)
