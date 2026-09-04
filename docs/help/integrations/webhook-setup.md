# Push webhook to Meta

Meta needs a publicly-reachable HTTPS URL to POST WhatsApp events to. The integration row exposes `webhook_url` (e.g. `https://app.example.com/webhooks/whatsapp/01J…`). Two ways to give it to Meta.

## Option A — Push to Meta button (recommended in dev)

The **Push to Meta** button on the integration's Details tab does two things:

1. Calls `GET /api/v1/tools/ngrok-tunnel` — the backend queries the local ngrok agent's inspector API (`http://127.0.0.1:4040`) and returns the first `https` tunnel URL. Empty result means "not detected — paste the URL manually".
2. Calls `POST /api/v1/integrations/{id}/webhook` with the detected (or manually pasted) URL. The backend:
   - Appends `/webhooks/whatsapp/{integration_id}`.
   - Reuses the stored `verify_token`.
   - POSTs the `webhook_configuration` override to Meta's `/{waba_id}` endpoint.
3. Returns the fully-qualified URL that was pushed so the UI can render it.

You never type the verify token again — it comes from `integration_credentials`.

## Option B — Manual paste into Meta

If you're not using ngrok or you'd rather do it yourself:

1. Copy `webhook_url` from the integration row.
2. developer.facebook.com → App → **WhatsApp → Configuration → Callback URL** → paste the URL.
3. Paste the same **verify token** you saved when creating the integration.
4. Click **Verify and save**. Meta hits `GET /webhooks/whatsapp/{id}?hub.mode=subscribe&hub.verify_token=…&hub.challenge=…` — Nudgeway echoes back `hub.challenge` if the token matches. Mismatch → `403`.
5. Under **Webhook fields**, subscribe to at minimum: `messages`, `message_status`, `calls`, `call_settings_update`.

## API

### Detect ngrok tunnel

```
GET /api/v1/tools/ngrok-tunnel
```

Response `200`:

```json
{ "public_url": "https://a1b2.ngrok-free.app" }
```

Empty string = not detected. Requires `integrations.manage`.

### Push webhook to Meta

```
POST /api/v1/integrations/{id}/webhook
Content-Type: application/json

{ "public_url": "https://a1b2.ngrok-free.app" }
```

Response `200`:

```json
{
  "webhook_url": "https://a1b2.ngrok-free.app/webhooks/whatsapp/01J…",
  "verify_token": "a0ea12cb…"
}
```

The `verify_token` in the response is convenience so a console that wants it pasted separately (some do) can grab it. It is **not** returned by any other endpoint.

Requires CSRF + `integrations.manage`. Additional statuses: `422` (malformed URL, non-`https` scheme), `502` (Meta rejected the `webhook_configuration` write).

## Production signature enforcement

Nudgeway's webhook ingress supports two verification modes controlled by `webhook.Ingress.RequireSignature`:

- **HMAC** (default and required in production) — verifies Meta's `X-Hub-Signature-256` header against the raw body using the integration's stored `app_secret`.
- **Payload-claims fallback** (dev only) — verifies `phone_number_id` + `waba_id` in the payload against `integration.Config`. Useful when the App Secret is unreliable (Meta test apps sometimes rotate silently).

**Set `NUDGEWAY_REQUIRE_SIGNATURE=1` in every non-dev environment.** With it unset, a forged body that names the right `phone_number_id` will be accepted.

## MCP

| operationId | Purpose |
|---|---|
| `setIntegrationWebhook` | Push a public URL to Meta as the WABA webhook override. |
| `getNgrokTunnel` | Best-effort ngrok tunnel discovery from the local agent. |

## Troubleshooting

- **Meta returns 403 on Verify and save** — the verify token in Meta's console does not match `integration_credentials.verify_token`. Re-paste from the value you saved.
- **"tunnel not detected"** — ngrok isn't running, or is on a non-default port. Start `ngrok http 8080` and retry, or paste the URL manually.
- **Push returns `422 non-https scheme`** — Meta only accepts `https://`. Cloudflared / ngrok give you HTTPS by default; a raw `http://localhost` won't work.
- **Deliveries land but are rejected with 401** — you're in production without `NUDGEWAY_REQUIRE_SIGNATURE=1` and the App Secret does not match. Set the env var and rotate.

## Related

- [Integrations overview](/#/integrations/overview)
- [Connect a WhatsApp integration](/#/integrations/connect-whatsapp)
- [First run](/#/getting-started/first-run)
- Source of truth: `docs/flows/webhook-ingestion.md`.
