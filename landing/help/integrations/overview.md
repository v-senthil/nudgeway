# Integrations

An **Integration** is one connection to a provider. For WhatsApp today, one integration = **one Meta Phone Number ID** — if you run three phone numbers across two WABAs, you have three integrations.

Config (non-secret) and secrets are stored separately: `Integration.config` is a JSON column with `phone_number_id` + `waba_id`; credentials (`access_token`, `app_secret`, `verify_token`) live envelope-encrypted in a companion `integration_credentials` row. Secrets are **never** returned in any API response.

## What ships today

| Provider | Type | Kind |
|---|---|---|
| `whatsapp` | `channel` | Meta WhatsApp Business Cloud API |

Other provider slots (`zoho_desk`, `openai`, …) are modelled in the domain but not implemented yet.

## Status lifecycle

| Status | Meaning |
|---|---|
| `connected` | Health check green. Workers use this integration. |
| `disconnected` | Soft-disconnected via `deleteIntegration`. Workers skip it. Row + credentials preserved for audit. |
| `degraded` | Health check failed on the last attempt. Workers may still try; provider-call log has the reason. |
| `auth_failed` | Meta rejected the access token. |
| `rate_limited` | Meta throttled the account. |

The MySQL enum (`pending | active | error | disabled`) is intentionally narrower than the domain vocabulary; the infrastructure layer maps between the two.

## Envelope encryption

Secrets are sealed with AES-256-GCM under a single 32-byte KEK loaded from `auth.credential_kek_hex` in `config/local.yaml`. Framing:

```
byte 0        version = 1
bytes 1..12   96-bit GCM nonce
bytes 13..end ciphertext || 128-bit GCM auth tag
```

Generate a KEK once:

```bash
openssl rand -hex 32
```

Rotate by re-encrypting all credential rows under the new KEK (CLI helper TBD; do it via SQL for now).

## Multi-tenancy

Every query includes `org_id` — the client cannot pass or influence it. Posting a WhatsApp-signed webhook body to a different tenant's integration id returns 404.

## Pages

- [Connect a WhatsApp integration](/#/integrations/connect-whatsapp)
- [Test the connection](/#/integrations/test-connection)
- [Push webhook to Meta](/#/integrations/webhook-setup)
- [Business profile](/#/integrations/business-profile)
- [Call settings](/#/integrations/call-settings)
- [Official Business Account (OBA)](/#/integrations/oba-status)
- [Business username](/#/integrations/business-username)
- [Phone number details + QR](/#/integrations/phone-number-details)
- [Delete an integration](/#/integrations/delete-integration)

Source of truth: `docs/domain/integration.md`, `skills/nudgeway-integrations/SKILL.md`.
