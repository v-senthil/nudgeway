# Domain — Integration + WebhookEvent

The `integration` domain package models the tenant-scoped configuration
for a concrete provider instance (a specific WhatsApp phone number, an
OpenAI key, etc.) and the raw webhook deliveries those integrations
produce.

## Types

### `integration.Integration`

Fields:

| Field | Type | Notes |
|-------|------|-------|
| `ID` | `integration.ID` | ULID string, stored as `VARBINARY(16)` |
| `OrgID` | `organization.ID` | tenant boundary |
| `Type` | `integration.Type` | `channel`, `ticketing`, `bot`, `ai`, `calling` |
| `Provider` | `string` | registry key: `whatsapp`, ... |
| `Name` | `string` | tenant-chosen display label |
| `Status` | `integration.Status` | `connected`, `disconnected`, `degraded`, `auth_failed`, `rate_limited` |
| `Config` | `map[string]any` | non-secret settings; JSON column |
| `CredentialsRef` | `[]byte` | opaque pointer to the credential row |
| `Capabilities` | `map[string]bool` | declared by the provider adapter |
| `Health` | `map[string]any` | last known probe result |
| `CreatedAt`, `UpdatedAt` | `time.Time` | UTC |

**Secrets are never on `Integration`.** Access tokens, app secrets,
verify tokens, refresh tokens live envelope-encrypted in a companion
`integration_credentials` row. Callers that need them ask
`mysql.Integrations.GetWithSecrets` explicitly.

The MySQL `integrations.status` ENUM (`pending`, `active`, `error`,
`disabled`) is intentionally narrower than the domain vocabulary; the
infrastructure layer maps between the two so the domain stays expressive
without breaking on-disk stability.

### `integration.WebhookEvent`

Fields:

| Field | Type | Notes |
|-------|------|-------|
| `ID` | `integration.WebhookEventID` | ULID |
| `OrgID` | `organization.ID` | tenant boundary |
| `IntegrationID` | `integration.ID` | which integration produced this delivery |
| `Provider` | `string` | echoed for cheap indexing |
| `ExternalEventID` | `string` | provider-supplied dedupe key |
| `ReceivedAt` | `time.Time` | ingress timestamp |
| `ProcessedAt` | `*time.Time` | set on final transition |
| `Status` | `integration.WebhookEventStatus` | `received`, `processing`, `processed`, `failed` |
| `RawBody` | `[]byte` | exact bytes we ACKed against |
| `Error` | `string` | populated on `failed` |

The `(IntegrationID, ExternalEventID)` tuple is UNIQUE. Duplicate
deliveries return `created=false` from `WebhookEventRepo.Insert` so the
ingress path collapses to a no-op ACK.

## Envelope encryption

Secret material is sealed with AES-256-GCM under a single 32-byte KEK
loaded from `auth.credential_kek_hex` in the config. The framing:

```
byte 0        version = 1
bytes 1..12   96-bit GCM nonce
bytes 13..end ciphertext || 128-bit GCM auth tag
```

APIs — `internal/infrastructure/crypto`:

- `crypto.ParseKEKHex(s string) ([]byte, error)` — 64 hex chars → 32 raw bytes.
- `crypto.NewEnvelope(kek []byte) (*Envelope, error)` — validated constructor.
- `(*Envelope).Encrypt(pt []byte) ([]byte, error)` — returns the framed blob.
- `(*Envelope).Decrypt(ct []byte) ([]byte, error)` — rejects unknown versions and truncated inputs.

Callers marshal a `map[string]string` of secret name → value to JSON and
pass the bytes to `Encrypt`. The `mysql.Integrations.GetWithSecrets`
helper reverses that on read.

`mysql.Bootstrap.EnsureIntegration(ctx, orgID, provider, name, cfg, secrets)`
wraps the entire insert-or-refresh flow (integration row + credentials
row + `credentials_ref` back-pointer) in a single transaction. It is
idempotent on `(org_id, provider, name)`.

## Migrations touching these tables

- `20260903000002_phase1_domain` — creates `integrations`,
  `integration_credentials`, `webhook_events`.
- `20260903000003_webhook_events_body` — adds `raw_body MEDIUMBLOB NULL`
  and relaxes `raw_ref` to `NULL` so callers can persist the inline body
  without going through the object-store indirection.

## Related repositories

- `repository.IntegrationRepo` — CRUD (secrets never returned).
- `repository.WebhookEventRepo` — `Insert` (idempotent), `MarkProcessed`,
  `MarkFailed`, `Get`, `ListPending`.
- MySQL impls: `mysql.NewIntegrations(db, env)`, `mysql.NewWebhookEvents(db)`.
