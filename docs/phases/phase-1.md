# Phase 1 — WhatsApp Inbox MVP

Status: **complete** (functionally). Real WhatsApp messages flow in and out through the canonical domain; media round-trips through HBase; status ticks advance live via WebSocket; BSUID rollout supported. See the Phase 1 CLOSE section of [`../../CHANGELOG.md`](../../CHANGELOG.md) for commit-level detail.

## Goal (from the master plan)

Real WhatsApp messages flow in and out through the canonical domain, visible in a real-time inbox.

## Exit criteria — all met

| Criterion | Evidence |
|---|---|
| Two agents in two browsers see an inbound WhatsApp message live | WebSocket bridge + `useInboxSocket` invalidates `['messages', <conversation_id>]` on `message.received` |
| Send replies from the browser | `POST /api/v1/messages` → Kafka → `SendWorker` → WhatsApp adapter → Meta accepts with a `wamid`, delivered on the customer's phone |
| Status ticks update in real time | Grey ✓ → grey ✓✓ → blue ✓✓ transitions via `SetProviderMessageID` + `UpdateStatusByProviderMessageID` + WS invalidation |
| Nothing in `application/` imports Meta types | Verified via `go-arch-lint` + grep guards |
| Media round-trip works | HBase-backed `attachments.Store`; Meta Media Upload API for outbound → media_id; direct-URL download for inbound → HBase → `<img>` |
| BSUID handled | `bsuid` identity persisted alongside phone; `MessageReceivedPayload.FromUserID` / `RecipientUserID` populated |

## Highlights of what shipped

### Backend
- **Wire-up**: `cmd/server/main.go` boots MySQL + Redis + Kafka + HBase + LocalFS fallback + Prometheus metrics. Worker pools (8× each for `webhook.process` + `message.send`) via `franz-go` consumer groups.
- **Domain**: `Contact`, `Identity`, `Session`, `Conversation`, `Message` with state-machine helpers. `Integration` + `WebhookEvent` types. `ContactIdentity` types include `phone`, `whatsapp`, `bsuid` (BSUID), `email`, `external`, `social`.
- **Repositories** (MySQL): 8 Phase 1 repos; `Messages` gains `SetProviderMessageID` and `UpdateStatusByProviderMessageID`; `BusinessEndpoints.Upsert`; `Conversations.ListForOrg` for the inbox list.
- **Attachments** (HBase): `internal/infrastructure/hbase/{client,schema,attachments}.go`. `LocalFS` retained as fallback. Row key = SHA-256; column families `d` (data) + `m` (metadata + per-integration Meta `media_id`).
- **Crypto**: `internal/infrastructure/crypto` — envelope encryption for integration credentials (AES-GCM per DEK, KEK from `auth.credential_kek_hex`).
- **Kafka**: `franz-go`-backed producer + consumer implementing `queue.Enqueuer` + `queue.Consumer` ports; publish path is fire-and-forget so REST stays sub-100 ms.
- **WebSocket**: `internal/infrastructure/websocket/{hub,room,client,bridge}.go`. Bridge subscribes to `MessageReceived / Sent / Delivered / Read / Failed / ConversationCreated / Updated / Assigned / Resolved` and broadcasts JSON frames to the org's connected browsers.
- **Provider adapter**: `internal/providers/whatsapp` — send/receive/mark-as-read/upload/download/verify-signature/parse-webhook; `channel.Provider` interface + `providers` registry.
- **Application services**: `SendService` (RequestSend + ProcessSend), `InboundService` (ProcessRaw), `ReadService` (MarkRead + MarkConversationRead), `IntegrationService` (List/Create/Test/Delete).
- **REST v1**: auth, integrations (CRUD + test), messages (send + list-by-conversation + mark-as-read), conversations (list), attachments (upload + serve), webhooks (Meta ingress + verify handshake).
- **Dev-mode webhook fallback**: `webhook.Ingress.RequireSignature` gate switches from HMAC to payload-claims match (`phone_number_id` + `waba_id` in `integration.Config`). `NUDGEWAY_REQUIRE_SIGNATURE=1` re-enables HMAC.

### BSUID rollout support
Meta is migrating from `wa_id` (phone-number-based) to BSUID (business-scoped user id, e.g. `IN.10173928811470384`). Full doc at `~/Documents/whatsapp_doc_tracker/docs/business-scoped-user-ids.md`.

- **Inbound**: mapper reads `contacts[].user_id`, `contacts[].parent_user_id`, `contacts[].profile.username`, `messages[].from_user_id`, `messages[].from_parent_user_id`. Statuses read `recipient_user_id`. `MessageReceivedPayload.FromUserID / FromParentUserID / FromUsername`; `MessageStatusPayload.RecipientUserID`.
- **Persistence**: `InboundService` upserts a `bsuid` `ContactIdentity` bound to the same Contact; promotes it to the Contact's `primary_identity_id` when the BSUID arrives.
- **Outbound**: `SendService.resolveRecipient` iterates identities preferring phone/wa_id today, BSUID as a fallback. TODO: promote BSUID to primary once Meta portfolio-side send accepts all BSUIDs universally.

### Frontend
- **Routes**: `/login`, `/inbox?c=<conversation_id>`, `/settings/integrations`.
- **Inbox**: 3-pane layout with conversation list (real data, sorted newest-first, live preview), thread with WhatsApp-style newest-at-bottom + auto-scroll, contact panel stub.
- **Composer**: text + emoji + attach button; file-picker uploads to `/api/v1/attachments`; preview strip; sends `media: {media_id, url, caption?, filename?}` in one call.
- **TickIcon** (SVG): three-dots (queued/sending), grey ✓ (sent), grey ✓✓ (delivered), blue ✓✓ (read), red ! (failed).
- **Rendering**: `TextBubble`, `MediaBubble` (image/video/audio/document/sticker), `LocationBubble`, `ContactCardBubble`, `InteractiveBubble`, `UnknownBubble`; reactions overlaid as chip on the target bubble.
- **Real-time**: `useInboxSocket` opens `/ws/inbox`, invalidates `['messages', <conversation_id>]` on any `message.*` frame.
- **Auto mark-as-read**: `Thread` fires `MarkConversationRead` on mount / conversation change with 5s throttle per conversation.
- **Header**: Nudgeway wordmark, org name, settings gear icon (opens `/settings/integrations`), user avatar dropdown with logout.
- **Settings**: Integrations page with WhatsApp connect wizard (form → test → save); status badges (Connected / Not connected / Pending / Degraded / Auth failed / Rate limited / Disabled / Unknown).

## Notable follow-ups deferred
- Contact 360 hydration is stubbed (right pane). Real profile / tags / custom fields / activity timeline lands in Phase 2.
- Frontend Settings only exposes Integrations. Templates / canned responses / automations / roles / audit ship in Phase 2 / 4.
- HBase namespace is currently the default; `nudgeway:attachments` awaits a gohbase namespace-RPC fix. Table name is `nudgeway_attachments` in the default namespace.
- BSUID promoted-to-primary on send once Meta portfolio-side accepts all BSUIDs (currently phone-first with BSUID fallback).
- The 30-day / 7-day media-URL expiry (Meta doc) — reconciler + refresh path lands with the archival worker in Phase 4.


## What shipped so far

### Phase 1 Task 0 — observability + infra checks (2026-09-04)

Replaces the Phase 0 placeholder metric handler with the real Prometheus surface and adds a Kafka readiness signal. Scope is deliberately narrow: **no `cmd/` wiring in this task** — the metric and probe APIs are ready; a follow-up commit will mount them on `/metrics` and `/readyz`.

**What landed**

- `internal/infrastructure/metrics/metrics.go` — dedicated `*prometheus.Registry`, canonical Nudgeway metric families (see the catalog below), and `Metrics.Handler()` serving the OpenMetrics exposition.
- `internal/infrastructure/metrics/http.go` — `Metrics.HTTPMiddleware(route)` — wraps a handler with a status-capturing `ResponseWriter` and records the HTTP counter + histogram.
- `internal/infrastructure/health/kafka.go` — `KafkaProbe(brokers)` — TCP dials each broker with a 500 ms timeout; green if any answers. Deeper metadata probe is Phase 2.
- `scripts/check-infra.sh` — Kafka section added: parses the inline `kafka.brokers` list from the config file (same style used for `hbase.zookeeper_quorum`) and TCP-checks each broker; `[skip]` when the key is absent.

**Metric families** (all names `nudgeway_<subsystem>_<name>_<unit>`; full catalog in `docs/observability/metric-catalog.md`)

| Family | Kind | Labels |
|---|---|---|
| `nudgeway_http_requests_total` | counter | method, path, status |
| `nudgeway_http_request_duration_seconds` | histogram | method, path, status |
| `nudgeway_provider_calls_total` | counter | provider, operation, outcome |
| `nudgeway_provider_call_duration_seconds` | histogram | provider, operation, outcome |
| `nudgeway_worker_jobs_total` | counter | lane, group, outcome |
| `nudgeway_worker_job_duration_seconds` | histogram | lane, group, outcome |
| `nudgeway_worker_job_retries_total` | counter | lane, group |
| `nudgeway_webhook_events_received_total` | counter | provider, integration_id |
| `nudgeway_kafka_producer_batch_bytes_total` | counter | topic |
| `nudgeway_kafka_consumer_lag_records` | gauge | topic, partition, group |
| `nudgeway_websocket_connections` | gauge | org_id_short |

Standard `GoCollector` and `ProcessCollector` are also registered on the same registry.

**Follow-ups**

- Wire `metrics.New()` in `cmd/server/main.go`, mount `m.Handler()` on `/metrics`, wrap the HTTP mux with `m.HTTPMiddleware(...)` per route template.
- Wire `health.KafkaProbe(cfg.Kafka.Brokers)` into the `/readyz` probe list.
- Have every provider / worker / websocket / kafka call site record onto its metric family.
- Phase 2: deeper Kafka probe (metadata fetch via the existing `franz-go` client) instead of a bare TCP dial.

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

### Webhook ingress (Phase 1 Task 2)

Provider-agnostic HTTP ingress mounted at `/webhooks/{provider}/{integration_id}` — the only place the transport-level intake for every future provider ever needs to live.

- **`internal/webhook/verifier.go`** — `SignatureVerifier` interface + `SignatureVerifierFunc` adapter + `StaticVerifierLookup` registry. The whatsapp adapter's existing `VerifySignature` free function is wrapped by cmd/server so the webhook package imports nothing provider-specific.
- **`internal/webhook/ingress.go`** — `Ingress` struct with deps `Secrets` (widens `IntegrationRepo` with `GetWithSecrets`), `WebhookEventRepo`, `queue.Enqueuer`, `VerifierLookup`, `*slog.Logger`. Pipeline: `MaxBytesReader(1 MiB)` → integration + secrets lookup (org derived from row) → provider verifier → `sha256(raw_body)` as external event id → `webhook_events` insert (duplicate = ACK 200) → enqueue on `webhook.process` lane → ACK 200. Every log line carries `request_id`, `provider`, `integration_id`, `org_id`, `event_id`, `sig_ok`, `dup`. Body-too-large returns 413 problem+json; unknown integration or provider-mismatch returns 404; bad signature returns 401.
- **`internal/webhook/ingress.go`** also exposes `VerifyHandshake` covering Meta's `GET` verification: `hub.mode=subscribe` + `hub.verify_token` matched against the integration's stored `verify_token` secret returns 200 `text/plain` echoing `hub.challenge`; anything else returns 403.
- **`internal/api/rest/v1/webhooks.go`** — mounts `GET` + `POST /webhooks/{provider}/{integration_id}` using Go 1.22 pattern matching + `r.PathValue`. Middleware chain is deliberately `RequestID → Recover → Logger` only; no `SessionAuth`, no `RequireCSRF` (external providers cannot present our cookies). Routes live outside `/api/v1/*`.
- **`internal/api/rest/v1/router.go`** — extended `Deps` with `Webhook WebhookDeps` and installs the webhook chain alongside the existing v1 auth routes.
- **OpenAPI** — `openapi.yaml` bumped to `0.2.0-phase1` with both endpoints spec'd (`security: []`, RFC 7807 problems for 401/404/413/500).

### Kafka event log + job queue (2026-09-04)

Ships the durable transport for `queue.Enqueuer` / `queue.Consumer` / `eventbus.Publisher` / `eventbus.Subscriber`. Supersedes the Redis-Streams portion of ADR 0004; see [ADR 0009](../adr/0009-kafka-for-event-log.md). Redis stays for cache, locks, rate limits, idempotency, presence.

- **`internal/infrastructure/kafka/client.go`** — `NewClient(cfg)` + `NewAdmin(cfg)` build franz-go + kadm clients. Producer defaults: idempotent, `acks=all`, snappy compression, 5 ms linger, 10k max buffered records.
- **`internal/infrastructure/kafka/topics.go`** — `TopicName(prefix, kind, name)` + `EnsureTopics(ctx, adm, factor, partitions, topics)`. Topic layout: `<prefix>.jobs.<lane>` and `<prefix>.events.<type>`. Idempotent — already-exists is a no-op.
- **`internal/infrastructure/kafka/producer.go`** — `NewProducer(client, prefix)` implements `queue.Enqueuer` (partition key = `Job.ID`, so upstream setting `Job.ID` to `conversation_id` preserves per-conversation ordering) and `eventbus.Publisher` (partition key = `Envelope.CorrelationID`). Synchronous produce so callers see broker errors.
- **`internal/infrastructure/kafka/consumer.go`** — `NewConsumer(cfg, logger)` implements `queue.Consumer`. Own franz-go client per lane per group. Auto-commit disabled; records commit only after handler returns nil. On handler error the batch stops committing so the partition rewinds on next poll — retry/backoff/DLQ live in `queue.Job`, not the transport.
- **`internal/infrastructure/kafka/eventbus.go`** — `NewEventBus(cfg, group, logger)` implements `eventbus.Subscriber`. First `Subscribe` for a Type starts a managed background goroutine; further subscribes for the same Type share it. `Close` cancels every goroutine and waits.
- **`internal/infrastructure/kafka/codec.go`** — JSON `EncodeJob` / `DecodeJob` / `EncodeEnvelope` / `DecodeEnvelope`. Protobuf is a follow-up.
- **`internal/infrastructure/kafka/probe.go`** — `Probe(client)` returns a `health.Probe` for `/readyz`.
- `KafkaConfig` added to `internal/infrastructure/config/config.go`: `Brokers`, `ClientID`, `TopicsPrefix`, `ReplicationFactor`, `DefaultPartitions`. YAML `kafka:` section + `NUDGEWAY_KAFKA_BROKERS` env override.
- `config/example.yaml` + `config/local.yaml` gain the `kafka:` section pointing at `127.0.0.1:9092`.

### WebSocket real-time — Task 6 (2026-09-04)

Live server → browser fan-out for canonical message + conversation events. Node-local `Hub` today; cross-node fan-out lands in Phase 2 without changing the wire contract.

- **`internal/infrastructure/websocket/hub.go`** — `Hub` owns per-org `Room`s. `NewHub(logger)` / `Room(orgID)` (get-or-create) / `Broadcast(orgID, payload)` (non-blocking per-client send; drop + counter on full queue so one slow tab cannot stall the fan-out) / `Close(ctx)` (graceful going-away on every client) / `Stats()` for future `/metrics` wiring.
- **`internal/infrastructure/websocket/room.go`** — `Room` with `sync.RWMutex`-guarded add / remove / snapshot; empty rooms retained for pointer stability.
- **`internal/infrastructure/websocket/client.go`** — one connection. Bounded goroutines: one read pump (drains + discards; server → client only in Phase 1) and one write pump (drains send queue, pings every `PingInterval` (default 25 s), writes bounded by `WriteTimeout` (default 10 s)). `SendBuffer` defaults to 64. `Client.Run(ctx)` blocks for the connection's lifetime and cleans up Room membership on exit.
- **`internal/infrastructure/websocket/bridge.go`** — `RegisterEventBridge(bus, hub, logger)` subscribes to `message.received`, `message.sent`, `message.delivered`, `message.read`, `message.failed`, `conversation.created`, `conversation.updated`, `conversation.assigned`, `conversation.resolved` and re-emits each as JSON `{type, org_id, occurred_at, correlation_id, payload}` onto the event's org room. Allow-list is explicit — new event types must be added here.
- **`internal/api/ws/inbox.go`** — `InboxHandler.ServeHTTP` requires a `Principal` on the request context (401 otherwise), calls `websocket.Accept` with a strict `OriginPatterns` allow-list (Vite dev `localhost:5173`, embedded prod `localhost:8080`, plus `127.0.0.1` + `[::1]` variants) and `InsecureSkipVerify=false`, then sends `{"type":"hello", "org_id", "user_id", "version":1}` before entering the pump loop.
- **`internal/api/rest/v1/router.go`** — `Deps.Hub` (+ optional `Deps.WSAllowedOrigins`); when set, `Mount` installs `GET /ws/inbox` directly on the mux (outside `/api/v1/*` so the Vite dev proxy can route `/ws` separately). Reuses the standard `RequestID → Recover → Logger → SessionAuth → RequireAuth → handler` chain.
- **Docs** — `docs/flows/websocket-realtime.md` with the full Mermaid (app → in-proc bus → bridge → hub → room → client → browser) plus wire-frame + back-pressure notes.

Wiring in `cmd/server/main.go` — construct `websocket.NewHub(logger)`, pass it as `Deps.Hub` to `v1.Mount`, and call `websocket.RegisterEventBridge(bus, hub, logger)` at boot. Left as a TODO for this commit so it lands atomically with the outbound send worker.

### Frontend UI — Task 7 (2026-09-04)

Replaces the Phase 0 placeholders with real, working UI backed by the peer-agent REST + WebSocket endpoints.

- **`web/src/lib/integrations.ts`** — hooks `useIntegrations`, `useCreateIntegration`, `useTestIntegration`, `useDeleteIntegration` (TanStack Query). Retries are skipped on 4xx to keep permission-denied and validation errors instantaneous.
- **`web/src/lib/messages.ts`** — hooks `useConversations`, `useConversationMessages`, `useSendMessage`. Response shape supports either `{items: []}` or a bare array (server tolerant).
- **`web/src/lib/events.ts`** — canonical event names (`message.received`, `message.sent`, `message.status`, `conversation.created`, `conversation.updated`, `integration.status`) + typed payload shapes + `isInboxFrame` guard.
- **`web/src/lib/ws.ts`** — `useInboxSocket(orgID)` opens a single shared WebSocket to `/ws/inbox` (relative — Vite proxy forwards to `:8080`). Exponential backoff with jitter (500 ms → 30 s). On each frame it invalidates the correct TanStack Query caches; `useSyncExternalStore` exposes status + last frame. `addInboxListener` is used by the Composer to reconcile optimistic sends against `message.sent`.
- **Settings → Integrations** (`web/src/routes/settings.integrations.tsx`, `web/src/features/settings/{IntegrationList,ConnectWhatsAppModal,IntegrationStatusBadge,DeleteConfirmModal}.tsx`) — TanStack Query–backed list with colored status badges. "Connect WhatsApp" modal captures `name`, `phone_number_id`, `waba_id`, `access_token`, `app_secret`, `verify_token`. On success the modal advances to a "Setup Meta webhook" step with copy-to-clipboard webhook URL + verify token (aria-live announcement). Per-row Test + Delete actions; delete guarded by a focus-trapped confirm modal.
- **Inbox** (`web/src/routes/inbox.tsx`, `web/src/features/inbox/{ConversationList,Thread,Composer,ContactPanel}.tsx`) — three-pane layout is now real. Conversation selection is stored in the URL as `?c=<id>`. Thread renders inbound-left / outbound-right bubbles with sending/sent/delivered/read/failed ticks and auto-scrolls on new messages. Composer sends via `POST /messages` with optimistic append keyed by `client_reference_id`; the WS `message.sent` / `message.status` frame reconciles the bubble id + status.
- All screens ship loading, empty, error, permission-denied and offline states. TS strict clean (`noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`), no `any`, no `as` casts outside API/DOM boundaries. Vite build ~96 kB gzipped.

### Inbound processing service + webhook worker — Task 3 (2026-09-04)

Async processing path that turns a verified webhook delivery into persisted domain state + fanned-out canonical events. Under `internal/application/message/`, `internal/webhook/`, and `internal/workers/`.

- **`internal/application/message/inbound.go`** — `InboundService`. Entry point `ProcessRaw(ctx, providerKey, integrationID, eventID, rawBody) error`. Flow: load integration + secrets (via `IntegrationRepo.GetWithSecrets`) → resolve `channel.Provider` via injected `ChannelProviderLookup` → `provider.ParseWebhook(ctx, nil, rawBody)` (headers=nil — signature already verified at ingress) → per envelope: `MessageReceived` upserts contact/identity/session/conversation and creates the message row (`UNIQUE(org, provider_message_id)` idempotency; duplicate-key errors swallowed as success); status callbacks advance the message via `MessageStatusByProviderID.UpdateStatusByProviderMessageID`; each envelope is republished on the injected `eventbus.Publisher`. Marks the `webhook_events` row `processed` / `failed`.
- **`internal/application/message/deps.go`** — `Deps` bundle for constructor injection. Local `IntegrationSecretsRepo` interface (matches `mysql.Integrations.GetWithSecrets`) and `MessageStatusByProviderID` supplement so the app layer stays port-only.
- **`internal/application/message/errors.go`** — sentinel errors + `Permanent(err) / IsPermanent(err)` classification; `IsDuplicateMessage(err)` predicate for UNIQUE-index absorption. Zero provider imports, zero infra imports.
- **`internal/webhook/lookup.go`** — process-level channel-provider registry. `RegisterProvider(key, p channel.Provider)` (called once at boot from `cmd/server`), `ProviderLookup(key) (channel.Provider, bool)`. Dependency-safe for the application layer — exposes only the port type.
- **`internal/workers/webhook_worker.go`** — `WebhookWorker.Run(ctx, consumer queue.Consumer, group string) error` subscribes to lane `webhook.process` and decodes each `queue.Job` into a `WebhookJobPayload{provider, integration_id, event_id, raw_body}` handed to `InboundService.ProcessRaw`. Malformed jobs are ACKed with an error log; transient errors are returned to the consumer for redelivery; permanent errors are already ACKed by the service so the queue does not retry forever.
- **`internal/workers/pool.go`** — `Pool{Name, Concurrency, Runner, Log}` spawns a bounded number of goroutines (default 1 when misconfigured). This is the ONLY sanctioned goroutine-spawning point per `CLAUDE.md` §11.

Failure semantics: permanent errors (integration missing, provider not registered, endpoint not provisioned, malformed envelope) → `webhook_events.MarkFailed` + ACK; transient errors (MySQL down, network, publisher failure) → `MarkFailed` + NACK for redelivery.

### Phase 1 Task 1 — data layer (shipped)

Persistence half of the Phase 1 skeleton — MySQL repositories for every Phase 1 entity, envelope-encryption for tenant secrets, the `Integration` and `WebhookEvent` domain types, and the ports the application will depend on.

- **Domain: `internal/domain/integration/{integration.go,webhook_event.go}`** — persisted `Integration` (Type, Provider, Name, Status, Config, opaque `CredentialsRef`, Capabilities, Health) and `WebhookEvent` (idempotency-keyed by `(IntegrationID, ExternalEventID)`). Secrets never live on the domain struct; only an opaque pointer.
- **Ports: `internal/ports/repository/{integration_repo.go,webhook_event_repo.go}`** — `IntegrationRepo` (Get/List/Create/Update/Delete) and `WebhookEventRepo` (`Insert` returns `(created=false, nil)` on the UNIQUE(integration_id, external_event_id) collision so duplicate deliveries collapse to a no-op ACK; plus `MarkProcessed`, `MarkFailed`, `Get`, `ListPending`).
- **Envelope crypto: `internal/infrastructure/crypto/{envelope.go,kek.go}`** — AES-256-GCM under a 32-byte KEK loaded from `auth.credential_kek_hex`. Framing: `[byte version=1][12B nonce][ciphertext||16B tag]`. Unknown versions and truncated ciphertexts are rejected. Public surface: `ParseKEKHex`, `NewEnvelope`, `(*Envelope).Encrypt`, `(*Envelope).Decrypt`.
- **MySQL implementations** — one file per port, matching the style of `mysql/users.go`:
  - `contacts.go` — `Upsert`, `Get`, `FindByPrimaryIdentity`, `List` (cursor + display-name substring).
  - `identities.go` — `FindOrCreate` via `INSERT ... ON DUPLICATE KEY UPDATE id=id` + read-back inside a single tx.
  - `business_endpoints.go` — `Get`, `FindByExternalID`, `List`.
  - `integrations.go` — `Get`, `List`, `Create`, `Update`, `Delete` (deletes the credential row first). Extra `GetWithSecrets` decrypts the linked `integration_credentials.ciphertext` and returns `map[string]string`. Domain-status ↔ DB-ENUM mapping isolated in `integrationStatusToDB` / `integrationStatusFromDB`.
  - `sessions_comm.go` — `FindOrCreateActive` relies on the STORED GENERATED `active_contact_id` UNIQUE index; concurrent inserts absorb the duplicate and reread.
  - `conversations.go` — `FindOrCreateOpen` returns newest open/pending/reopened row for the session or inserts.
  - `messages.go` — `Create` returns `ErrDuplicateMessage` on the (org, provider, provider_message_id) UNIQUE violation. `UpdateStatus` matches by internal ULID first, then falls back to `provider_message_id` so webhook status callbacks land idempotently.
  - `webhook_events.go` — `Insert` returns `(created=false, nil)` on 1062. `MarkProcessed` / `MarkFailed` / `Get` / `ListPending` round out the port.
  - `errors.go` — shared MySQL 1062 detector via `github.com/go-sql-driver/mysql.MySQLError`.
- **`Bootstrap.EnsureIntegration`** — idempotent on `(org, provider, name)`. Envelope-encrypts `secrets` into `integration_credentials.ciphertext` and (for `whatsapp` when `phone_number_id` is set) upserts a matching `business_endpoints` row. Requires `WithEnvelope(env)`.
- **Migration `20260903000003_webhook_events_body`** — adds `raw_body MEDIUMBLOB NULL` and relaxes `raw_ref` to `NULL` so callers can persist the exact ACKed bytes inline instead of going through an object-store indirection.
- **Docs** — `docs/domain/integration.md` documents the entity, envelope framing, and repo surface.

### Outbound send — Task 4 (2026-09-04)

The REST + queue + worker + application service pipeline for outbound sends
landed together. The provider adapter is invoked only from the worker,
never from the REST path.

- **Application service** — `internal/application/message/send.go`
  - `SendService.RequestSend` — validates the request, resolves
    `conversation → session → endpoint → integration` (all org-scoped),
    inserts `Message(QUEUED, direction=outbound)`, enqueues a
    `SendJobPayload` on `message.send`, and publishes
    `MessageSendRequested`. Never touches the provider adapter.
  - `SendService.ProcessSend` — resolves integration + decrypted secrets
    via `IntegrationSecrets.GetWithSecrets`, obtains the
    `channel.Provider` adapter via the `ProviderRegistry` port, calls
    `SendMessage(ctx, channel.SendRequest{IdempotencyKey=msg_id})`,
    updates message status to `sent` on success (publishing
    `MessageSent`), returns transient errors so the queue retries with
    backoff, marks `failed` and publishes `MessageFailed` on permanent
    errors.
- **DTOs** — `SendRequest`, `SendResponse`, `SendJobPayload` in
  `send_dto.go` — the last of which is JSON-encoded onto the queue.
- **REST handler** — `internal/api/rest/v1/messages.go`
  - `POST /api/v1/messages` (auth + CSRF) → `202 {message_id, status:"queued"}`.
  - `GET /api/v1/conversations/{id}/messages` (auth) — newest-first,
    cursor-based pagination.
  - `GET /api/v1/conversations` (auth) — placeholder empty list until the
    full listing lands.
- **Worker** — `internal/workers/send_worker.go` mirrors `WebhookWorker`:
  `Run(ctx, consumer, group)` → `Consume(appmsg.SendLane, group, handle)`.
- **OpenAPI** — spec at `0.2.0-phase1` gains `POST /api/v1/messages`,
  `GET /api/v1/conversations/{id}/messages`, `GET /api/v1/conversations`
  plus schemas `SendMessageRequest`, `SendMessageAccepted`, `Message`,
  `MessageList`, `Conversation`, `ConversationList`.
- **Docs** — `docs/flows/outbound-send.md` refreshed with the concrete
  queue lane + retry semantics + state machine.

### Integrations REST + CLI — Task 5 (2026-09-04)

Operators can now list, create, test, and disconnect integrations behind
`/api/v1/integrations/*` and seed a WhatsApp integration from the CLI.
Nothing in this task touches the domain model — the service dispatches
to concrete adapters purely through the `channel.Provider` port.

- **Application service** — `internal/application/integration/service.go`:
  - `List`, `Get`, `Create`, `Test`, `Delete`. Never returns credentials.
  - `providerSchema` table validates required config + secret keys per
    provider. The `whatsapp` entry requires `phone_number_id` +
    `waba_id` in `config`, and `access_token` + `app_secret` +
    `verify_token` in `secrets`. Unknown / unregistered providers are
    rejected at Create time via `providers.Lookup`.
  - Belt-and-braces filter strips any known-sensitive keys
    (`access_token`, `app_secret`, `verify_token`, `refresh_token`,
    `api_key`, `client_secret`) from the public view.
  - `Test` resolves the adapter via a `ProviderResolver` interface
    (implemented in `cmd/server`), calls `channel.Provider.HealthCheck`,
    then persists `Status` + `Health`.
  - `Delete` soft-disconnects (`Status = disconnected`); the row +
    ciphertext stay for the Phase 4 audit trail.
- **REST handler** — `internal/api/rest/v1/integrations.go`:
  five endpoints, all gated by `integrations.manage`; writes also
  require CSRF.
  - `GET /api/v1/integrations` — list.
  - `POST /api/v1/integrations` — create; response includes the
    fully-qualified `webhook_url` (`IntegrationsDeps.PublicBaseURL` +
    `/webhooks/<provider>/<id>`) for pasting into the provider console.
  - `GET /api/v1/integrations/{id}` — get one.
  - `POST /api/v1/integrations/{id}/test` — HealthCheck.
  - `DELETE /api/v1/integrations/{id}` — soft-disconnect.
  - Errors follow RFC 7807; `ErrValidation` → 422, `ErrNotFound` → 404.
- **CLI `integration create`** — new subcommand in `cmd/cli/main.go`:

  ```bash
  nudgeway-cli integration create \
    --org-slug acme --provider whatsapp --name "Acme Support" \
    --phone-number-id 123 --waba-id 456 \
    --access-token EAA... --app-secret ... --verify-token ...
  ```

  Calls `mysql.Bootstrap.EnsureIntegration` (idempotent on
  `(org, provider, name)`), reusing `crypto.ParseKEKHex` +
  `crypto.NewEnvelope` from the loaded config. Prints
  `integration created: id=..., webhook_url=/webhooks/whatsapp/<id>`.
- **OpenAPI** — spec bumped to `0.2.1-phase1`: new schemas
  `Integration`, `IntegrationList`, `CreateIntegrationRequest`,
  `TestIntegrationResponse` and the five paths above.

## Exit criteria (Phase 1)

- Two agents in two browsers see an inbound WhatsApp message appear live.
- They can send replies; `sent → delivered → read` ticks update in real time.
- Nothing in `application/` imports Meta types.
- Docs updated: phase-1 + flows + provider all reflect the shipped surface.
