# Phase 1 — WhatsApp Inbox MVP

Status: **in progress**. Foundation laid; ingress + workers + UI wizard pending.

## Goal (from the master plan)

Real WhatsApp messages flow in and out through the canonical domain, visible in a real-time inbox.

## What shipped so far

### Domain model (commit `c3ed4e2`)

Real types, not stubs — under `internal/domain/`:

| Package | Type | Highlights |
|--------|------|------------|
| `contact` | `Contact` | id, org, display name, avatar, primary identity id, last_seen |
| `identity` | `Identity`, `Type` enum, `NormalizePhoneE164`, `NormalizeEmail` | (org, provider, normalized_value) is the merge key |
| `session` | `Session`, `Status` enum, `Close`, `Reopen` | one ACTIVE per (org, endpoint, contact) enforced at the DB |
| `conversation` | `Conversation`, `Status` enum, `Assign`, `Resolve`, `Reopen`, `MarkRead` | status machine |
| `message` | `Message`, `Direction`, `Type`, `Status` enums, `Transition()` | QUEUED→SENT→DELIVERED→READ; FAILED terminal; enforced by helper |
| `message.payload` | `TextPayload`, `MediaPayload`, `TemplatePayload`, `InteractivePayload`, `LocationPayload`, `ContactsPayload`, `ReactionPayload` | provider-neutral |
| `events` (new file `payloads.go`) | `MessageReceivedPayload`, `MessageStatusPayload` | for the canonical events emitted by the WhatsApp webhook parser |

### Repository ports (commit `c3ed4e2`)

Interfaces the application will depend on — under `internal/ports/repository/`:

- `ContactRepo` — Upsert, Get, FindByPrimaryIdentity, List (org-scoped, paginated)
- `IdentityRepo` — FindOrCreate(org, provider, normalizedValue), ListForContact
- `BusinessEndpointRepo` — Get, FindByExternalID, List (used to resolve which WhatsApp phone number a webhook targets)
- `SessionRepo` — FindOrCreateActive, Get, Close
- `ConversationRepo` — FindOrCreateOpen, Get, UpdateStatus, Assign, ListForContact
- `MessageRepo` — Create, UpdateStatus, ListByConversation

### WhatsApp Cloud API adapter (commit `c3ed4e2`)

Full `channel.Provider` implementation under `internal/providers/whatsapp/`:

- **`provider.go`** — self-registers via `init()` into the provider registry as `KindChannel/"whatsapp"`.
- **`client.go`** — retrying Graph API HTTP client; jittered exponential backoff (250 ms → 5 s); classifies errors into `Transient` / `RateLimited` / `Auth` / `Permanent`.
- **`webhook.go`** — `VerifySignature` (constant-time HMAC-SHA256 of `X-Hub-Signature-256`); `ParseWebhook` covers the full documented inbound surface.
- **`mapper.go`** — canonical ⇄ Meta payload conversion covering `text`, `image`, `video`, `audio`, `document`, `sticker`, `location`, `contacts`, `interactive` (button_reply + list_reply), `button` (template reply), `reaction`, and a preserving `unknown` fallback that stashes the raw JSON in metadata (future-proof).
- **`templates.go`** — template CRUD via WABA API.
- **`media.go`** — media download by ID (URL lookup + bearer GET).
- **`capabilities.go`** — reports `SendText`, `SendMedia`, `SendTemplate`, `ReceiveMessages`, `Templates`, `Flows` = true; `Groups`, `Calls` = false (Phase 3+).
- **`errors.go`** — error classification helpers.

Reference discipline: source of truth is the local Meta docs mirror at `~/Documents/whatsapp_doc_tracker/docs/`. Zero invented Meta APIs.

### Migration 20260903000002_phase1_domain (commit `c3ed4e2`)

Nine tables, all InnoDB / utf8mb4, all non-primary indexes org-first:

- `contacts` (mutual FK to `contact_identities.id` for `primary_identity_id`)
- `contact_identities` — UNIQUE(org_id, provider, normalized_value)
- `business_endpoints` — UNIQUE(org_id, provider, external_id)
- `integrations` — provider-instance config per tenant
- `integration_credentials` — envelope-encrypted secrets (KEK from `auth.credential_kek_hex`)
- `sessions_comm` — STORED GENERATED `active_contact_id` column + UNIQUE index enforces "one ACTIVE session per (org, endpoint, contact)"
- `conversations` — indexes on `(org_id, status, last_message_at)`
- `messages` — UNIQUE(org_id, provider, provider_message_id) for idempotent status updates
- `webhook_events` — UNIQUE(integration_id, external_event_id) for idempotent ingestion

### Docs (commit `c3ed4e2`)

- `docs/domain/{contact,identity,session,conversation,message}.md` — one per entity, with invariants + state-machine Mermaid.
- `docs/providers/whatsapp.md` — capability matrix, config schema, mapped operations, webhook event mapping, rate limits, error classification, TODO list.
- `docs/flows/{inbound-message,outbound-send,webhook-ingestion}.md` — Mermaid sequence diagrams.

## What's pending in Phase 1

- **Webhook ingress route** — `POST /webhooks/whatsapp/:integration_id` — sig verify → persist raw event → ACK 200 → enqueue.
- **Redis Streams queue** — `queue.Enqueuer` + `queue.Consumer` implementations on Redis Streams with per-lane consumer groups.
- **Webhook worker** — consumes `q:webhook.process`, dispatches to `whatsapp.ParseWebhook`, upserts Contact/Identity/Session/Conversation, persists Message, publishes `MessageReceived`.
- **MySQL implementations** of the Phase 1 repository ports.
- **Envelope crypto** — `internal/infrastructure/crypto/envelope.go` (AES-GCM using the KEK from config).
- **Integration REST API** — `GET/POST /api/v1/integrations`, `POST /api/v1/integrations/:id/test`, `DELETE /api/v1/integrations/:id`.
- **Integration wizard UI** — replace the "Coming Soon" placeholder on `/settings/integrations` with a real WhatsApp connection form (phone number id, WABA id, access token, app secret, verify token) + test button + display of the webhook URL to paste into Meta.
- **Outbound send** — `POST /api/v1/messages` → persist QUEUED → enqueue → `message.send` worker → adapter.SendMessage → status update → publish `MessageSent`.
- **WebSocket real-time** — inbox subscribes to `message.received` / `message.sent` events for the current org and updates the conversation list + thread live.

## Exit criteria (Phase 1)

- Two agents in two browsers see an inbound WhatsApp message appear live.
- They can send replies; `sent → delivered → read` ticks update in real time.
- Nothing in `application/` imports Meta types.
- Docs updated: phase-1 + flows + provider all reflect the shipped surface.
