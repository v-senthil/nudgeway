# fullWA CHANGELOG

Human-readable project history. Commit hashes are on `main` at `v-senthil/whatsapp-cloud-api`. For the OpenAPI-specific changelog see [`docs/api/CHANGELOG.md`](docs/api/CHANGELOG.md).

Format: reverse-chronological. Latest at the top.

---

## 2026-09-04 — Phase 1 Task 5: integrations API + CLI

Operators can list / create / test / disconnect provider integrations behind `/api/v1/integrations/*`, and a new `fullwa-cli integration create` subcommand seeds a WhatsApp integration without touching the UI. Secret material is envelope-encrypted at rest and never crosses the API boundary — the response's `webhook_url` is the tenant-facing URL to paste into the provider console.

- **`internal/application/integration/service.go`** — `Service` with `List`, `Get`, `Create`, `Test`, `Delete`. A small `providerSchema` map validates required config + secret keys per provider (whatsapp: `phone_number_id` + `waba_id` in config; `access_token` + `app_secret` + `verify_token` in secrets). Unknown / unregistered providers rejected via `providers.Lookup`. `Test` dispatches to `channel.Provider.HealthCheck` through a `ProviderResolver` interface (implemented in `cmd/server`, the only package allowed to import concrete adapters) and persists `Status` + `Health`. `Delete` soft-disconnects so the audit trail survives.
- **`internal/application/integration/dto.go`** — `CreateInput`, `TestResult`, `PublicIntegration` (secrets-stripped view + `webhook_url`).
- **`internal/api/rest/v1/integrations.go`** — `GET /api/v1/integrations`, `POST /api/v1/integrations`, `GET /api/v1/integrations/{id}`, `POST /api/v1/integrations/{id}/test`, `DELETE /api/v1/integrations/{id}`. All auth-gated (session cookie) + `integrations.manage`; writes require CSRF. Errors are RFC 7807 — validation → 422, not found → 404.
- **`internal/api/rest/v1/router.go`** — new `Integrations IntegrationsDeps` field on `Deps`; `mountIntegrations` invoked when `Service` is non-nil.
- **`cmd/cli/main.go`** — new `integration create` subcommand: `--org-slug`, `--provider whatsapp`, `--name`, `--phone-number-id`, `--waba-id`, `--access-token`, `--app-secret`, `--verify-token`. Calls `mysql.Bootstrap.EnsureIntegration` (idempotent on `(org, provider, name)`), builds the envelope from `auth.credential_kek_hex`, prints `integration created: id=..., webhook_url=/webhooks/whatsapp/<id>`.
- **`internal/infrastructure/mysql/bootstrap.go`** — `Bootstrap.EnsureIntegration(ctx, orgID, provider, name, cfg, secrets)`: seeds `integrations` (status `pending`), envelope-encrypts `secrets` into `integration_credentials.ciphertext` (`ON DUPLICATE KEY UPDATE`), and upserts a matching `business_endpoints` row for channel-kind providers on the `(org, provider, external_id)` unique key. Requires `WithEnvelope(env)`.
- **OpenAPI** — bumped to `0.2.1-phase1`: schemas `Integration`, `IntegrationList`, `CreateIntegrationRequest`, `TestIntegrationResponse`; five new paths.
- **Docs** — `docs/api/CHANGELOG.md` entry, `docs/phases/phase-1.md` moves Integrations REST + CLI to shipped, `docs/providers/whatsapp.md` gains a Provisioning section covering the CLI + form fields.

---

## 2026-09-04 — Phase 1 Task 4: outbound send

The REST → persist → enqueue → worker → provider pipeline for outbound messages. Agents (and later automations) can now `POST /api/v1/messages`, get a 202 with a canonical message ID, and the worker asynchronously calls the WhatsApp adapter. Status transitions (`queued → sent → delivered → read`) fan out via canonical events; failures classify as retryable (transport / rate-limit) or permanent (auth / validation), mirroring the WhatsApp `APIError.Retryable()` contract without the application layer knowing about Meta.

- **`internal/application/message/send.go`** — `SendService.RequestSend` validates, resolves `conversation → session → endpoint → integration` (all org-scoped), inserts `Message(QUEUED, direction=outbound)`, encodes a `SendJobPayload` on the `message.send` lane, and publishes `MessageSendRequested`. Never touches the provider adapter. `SendService.ProcessSend` (invoked by the worker) resolves integration + decrypted secrets via `IntegrationSecrets.GetWithSecrets`, looks up the `channel.Provider` through the `ProviderRegistry` port, calls `SendMessage` with the message ID as the idempotency key, updates status to `sent` on success (publishing `MessageSent`), returns transient errors so the queue retries with backoff, marks `failed` and publishes `MessageFailed` on permanent errors.
- **`internal/application/message/send_dto.go`** — DTOs: `SendRequest`, `SendResponse`, `SendJobPayload`. Nothing provider-specific.
- **`internal/api/rest/v1/messages.go`** — `POST /api/v1/messages` (auth + CSRF) returns 202 `{message_id, status:"queued"}`. `GET /api/v1/conversations/{id}/messages` returns cursor-paginated messages, newest first. `GET /api/v1/conversations` is a Phase-1 placeholder empty list.
- **`internal/api/rest/v1/router.go`** — mount call added; the existing base + authed chain builders are reused so the middleware order stays `RequestID → Recover → Logger → SessionAuth → RequireAuth → RequireCSRF`.
- **`internal/workers/send_worker.go`** — mirrors `WebhookWorker`. `Run(ctx, consumer, group)` → `Consume(appmsg.SendLane, group, handle)`. Malformed payloads are ACKed (permanent); transient errors are returned for redelivery.
- **OpenAPI** — spec at `0.2.0-phase1` gains `POST /api/v1/messages`, `GET /api/v1/conversations/{id}/messages`, `GET /api/v1/conversations`, and schemas `SendMessageRequest`, `SendMessageAccepted`, `Message`, `MessageList`, `Conversation`, `ConversationList`.
- **Docs** — `docs/flows/outbound-send.md` refreshed with the concrete Kafka lane name (`message.send`), the retry / classification semantics, and the full state machine.

---

## 2026-09-04 — Phase 1 Task 3: inbound processing service + webhook worker

Async pipeline that turns a signature-verified webhook delivery into persisted domain state + fanned-out canonical events. Ships the last runtime piece needed to see a real WhatsApp inbound message land in the inbox (paired with Agent A's `IntegrationRepo.GetWithSecrets` + `WebhookEventRepo`, Agent B's Kafka `queue.Consumer`, and Agent C's ingress).

- **`internal/application/message/inbound.go`** — `InboundService.ProcessRaw(ctx, providerKey, integrationID, eventID, rawBody) error`. Loads integration + secrets (org_id), resolves the `channel.Provider` via `webhook.ProviderLookup` (registry indirection so the app layer imports no provider adapter), calls `provider.ParseWebhook(ctx, nil, rawBody)`, then per envelope: `MessageReceived` upserts contact / identity / session / conversation and creates the message row (duplicate-key on `UNIQUE(org, provider, provider_message_id)` swallowed as success); status callbacks (`sent` / `delivered` / `read` / `failed`) advance the message via the supplemental `MessageStatusByProviderID` port; every envelope is republished on the injected `eventbus.Publisher`. The webhook_events row is marked `processed` / `failed`. No DB transaction spans the provider call.
- **`internal/application/message/deps.go`** — `Deps` bundle for constructor injection. Local `IntegrationSecretsRepo` interface + `MessageStatusByProviderID` supplement so the application layer stays port-only. `ChannelProviderLookup` is a plain function type so the service imports nothing from `internal/webhook`.
- **`internal/application/message/errors.go`** — `Permanent(err)` / `IsPermanent(err)` classification + `IsDuplicateMessage(err)` for UNIQUE-index absorption. Sentinel errors: `ErrIntegrationNotFound`, `ErrProviderNotRegistered`, `ErrEndpointNotProvisioned`, `ErrUnknownEnvelope`.
- **`internal/webhook/lookup.go`** — process-level channel-provider registry. `RegisterProvider(key string, p channel.Provider)` (called once at boot from `cmd/server`), `ProviderLookup(key) (channel.Provider, bool)`. `UnregisterProvider` for tests. Concurrency-safe via `sync.RWMutex`.
- **`internal/workers/webhook_worker.go`** — `WebhookWorker.Run(ctx, consumer queue.Consumer, group string) error` subscribes to lane `webhook.process`. Decodes each `queue.Job.Payload` as a `WebhookJobPayload{provider, integration_id, event_id, raw_body}` and calls `InboundService.ProcessRaw`. Malformed jobs are ACKed with an error log (never redelivered). Transient errors return to the consumer for redelivery per backoff; permanent errors were already ACKed by `ProcessRaw`.
- **`internal/workers/pool.go`** — `Pool{Name, Concurrency, Runner, Log}.Run(ctx)` spawns a bounded number of goroutines running the given `Runner` (interface + `RunnerFunc` adapter). Concurrency ≤0 clamps to 1 so misconfiguration cannot silently disable a worker. This is the only sanctioned goroutine-spawning point in the codebase.
- **Docs** — `docs/flows/inbound-message.md` regenerated with the concrete Mermaid (pool → worker → InboundService → per-envelope branches → mark processed/failed). `docs/phases/phase-1.md` moves "Webhook worker" from pending to shipped and enumerates the new files.

Failure semantics summary: permanent (integration missing, provider not registered, endpoint not provisioned, malformed envelope) → `webhook_events.MarkFailed` + ACK the queue job; transient (MySQL down, network, publisher failure) → `MarkFailed` + return the error so the consumer redelivers.

---

## 2026-09-04 — Phase 1 Task 7: frontend UI (integrations wizard, real-time inbox)

Replaces the Phase 0 "Coming Soon" placeholders with real, working UI backed by the peer-agent REST + WebSocket endpoints. Working end-to-end path from "connect WhatsApp" → "see conversations" → "send a reply" → "live status ticks".

- **Settings → Integrations** (`web/src/routes/settings.integrations.tsx` + `web/src/features/settings/*`) — TanStack Query–backed list with colored status badges (connected / degraded / auth_failed / pending / disabled). "Connect WhatsApp" modal collects `name`, `phone_number_id`, `waba_id`, `access_token`, `app_secret`, `verify_token` and calls `POST /api/v1/integrations`. On success a second step shows the webhook URL + verify token with copy-to-clipboard buttons and an aria-live announcement. Per-row Test (`POST /integrations/{id}/test`) and Delete (`DELETE /integrations/{id}`) actions, delete guarded by a focus-trapped confirm modal.
- **Inbox** (`web/src/routes/inbox.tsx` + `web/src/features/inbox/*`) — three-pane layout now real. `ConversationList` fetches `GET /conversations`, supports client-side search, and stores selection in the URL as `?c=<id>`. `Thread` fetches `GET /conversations/{id}/messages`, renders inbound-left / outbound-right bubbles with message-type-aware rendering (text now; media placeholder), auto-scrolls on new messages, and shows sending / sent / delivered / read / failed ticks. `Composer` sends via `POST /messages` with optimistic append keyed by `client_reference_id`; the WS `message.sent` frame reconciles the optimistic bubble.
- **WebSocket hook** (`web/src/lib/ws.ts`) — `useInboxSocket(orgID)` opens a single shared connection to `/ws/inbox` (relative URL — Vite proxy forwards to `:8080`). Exponential backoff with jitter (500 ms → 30 s). On each frame it invalidates the correct TanStack Query caches: `message.received` / `message.sent` / `message.status` → `['messages', conversation_id]` + `['conversations']`; `conversation.created` / `conversation.updated` → `['conversations']`; `integration.status` → `['integrations']`. A `useSyncExternalStore` snapshot exposes status + last frame; `addInboxListener` lets the Composer reconcile optimistic sends.
- All screens ship loading, empty, error, permission-denied and offline states. TypeScript strict clean (`noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`), no `any`, no `as` casts outside API/DOM boundaries. Vite build ~96 kB gzipped.

---

## 2026-09-04 — Phase 1 Task 6: WebSocket real-time

Live server → browser fan-out for canonical message + conversation events. Node-local hub, per-org rooms, non-blocking broadcast with drop counting so a single slow tab cannot stall the fan-out. Cross-node fan-out is a Phase 2 concern; the wire contract with the browser will not change when that lands.

- **Phase 1 Task 6 — WebSocket real-time**
- `internal/infrastructure/websocket/{hub,room,client}.go` — `Hub` with per-org `Room`s and per-`Client` bounded send queues (default 64). `nhooyr.io/websocket` transport; two bounded goroutines per connection (read pump + write pump); pings every 25 s; write timeout 10 s. `Broadcast` snapshots the room under a read lock and pushes non-blocking onto each client's channel — full queues drop + count instead of stalling.
- `internal/infrastructure/websocket/bridge.go` — `RegisterEventBridge(bus, hub, logger)` subscribes to `message.{received,sent,delivered,read,failed}` and `conversation.{created,updated,assigned,resolved}` and re-emits each as JSON `{type, org_id, occurred_at, correlation_id, payload}` onto the event's org room. The bridged type list is an explicit allow-list.
- `internal/api/ws/inbox.go` — `InboxHandler.ServeHTTP` requires a `Principal` (401 otherwise), calls `websocket.Accept` with a strict `OriginPatterns` allow-list (Vite dev `localhost:5173`, embedded prod `localhost:8080`, plus `127.0.0.1` + `[::1]` variants) and `InsecureSkipVerify=false`, then sends `{"type":"hello", "org_id", "user_id", "version":1}` before entering the pump loop.
- `internal/api/rest/v1/router.go` — `Deps.Hub` (+ optional `Deps.WSAllowedOrigins`); when set, `Mount` installs `GET /ws/inbox` directly on the mux (outside `/api/v1/*` so the Vite dev proxy can route `/ws` separately). Reuses the standard `RequestID → Recover → Logger → SessionAuth → RequireAuth → handler` chain.
- `docs/flows/websocket-realtime.md` — new. Full Mermaid sequence (app → in-proc bus → bridge → hub → room → client → browser) plus wire-frame + back-pressure notes.
- `docs/phases/phase-1.md` — Task 6 section under "What shipped so far"; `WebSocket real-time` removed from the pending list and replaced with the follow-up "Frontend WS client" line.
- `go.mod` — `nhooyr.io/websocket v1.8.17` added.

Follow-up (out of scope for this commit): wire `websocket.NewHub(logger)` + `RegisterEventBridge(bus, hub, logger)` from `cmd/server/main.go`, and land a `web/src/lib/ws.ts` client with auto-reconnect that hydrates the TanStack Query cache.

---

## Phase 1 Task 1 — data layer

- **Domain: `integration.Integration` + `integration.WebhookEvent`** — canonical persisted types for tenant-scoped provider instances and raw webhook deliveries. Secrets never live on `Integration`; only an opaque `CredentialsRef` pointer.
- **Ports: `IntegrationRepo`, `WebhookEventRepo`** — `WebhookEventRepo.Insert` returns `(created=false, nil)` on the UNIQUE(integration_id, external_event_id) collision so duplicate deliveries collapse to no-ops.
- **Envelope crypto: `internal/infrastructure/crypto`** — AES-256-GCM with a 32-byte KEK. Framing is `[version=1][12B nonce][ciphertext||16B tag]`; unknown versions are rejected. `ParseKEKHex` decodes 64-hex-char config values.
- **MySQL repositories** for every Phase 1 entity — contacts, identities (`FindOrCreate` via `INSERT ... ON DUPLICATE KEY UPDATE`), business endpoints, integrations (`GetWithSecrets` decrypts on demand), sessions_comm (uses STORED GENERATED `active_contact_id` UNIQUE index to claim atomically), conversations (`FindOrCreateOpen`), messages (`ErrDuplicateMessage` on the org+provider+provider_message_id UNIQUE; `UpdateStatus` matches by internal id then falls back to provider_message_id), webhook_events.
- **`Bootstrap.EnsureIntegration`** — idempotent on `(org, provider, name)`; envelope-encrypts secrets and links `integrations.credentials_ref` in the same tx.
- **Migration `20260903000003_webhook_events_body`** — adds `raw_body MEDIUMBLOB NULL` and relaxes `raw_ref` to `NULL` so callers can persist the inline body without going through the object-store indirection.
- **Docs:** new `docs/domain/integration.md`; phase-1 status page updated.

## 2026-09-04 — Phase 1 Task 0: observability + infra checks

Real Prometheus metrics and a Kafka readiness signal replace the Phase 0 placeholders. No behaviour change to existing routes; new surfaces are ready to be wired at `cmd/server/main.go` (`/metrics`, `/readyz`) — the wiring change is a follow-up commit because Task 0 keeps `cmd/` untouched.

- **Phase 1 Task 0 — observability + infra checks**
- `internal/infrastructure/metrics/metrics.go` — dedicated `*prometheus.Registry` plus the fullWA canonical metric families: HTTP requests + latency, provider calls + latency, worker jobs + latency + retries, webhook events received, Kafka producer batch bytes + consumer lag, WebSocket connections. Registers `GoCollector` + `ProcessCollector`. `Metrics.Handler()` serves the OpenMetrics exposition.
- `internal/infrastructure/metrics/http.go` — small `HTTPMiddleware(route)` that wraps a handler with a status-capturing `ResponseWriter` and records the HTTP counter + histogram.
- `internal/infrastructure/health/kafka.go` — `KafkaProbe(brokers)` — 500 ms per-broker TCP dial; ok if at least one broker answers. Deeper metadata probe is Phase 2.
- `scripts/check-infra.sh` — new Kafka section reads inline `kafka.brokers` list (matching the `hbase.zookeeper_quorum` style) and TCP-checks each; green if any responds; `[skip]` when the key is absent.
- Deps: `github.com/prometheus/client_golang` v1.24.1 added.
- Docs: `docs/phases/phase-1.md` (Task 0 section), `docs/runbook.md` (Metrics section), `docs/observability/metric-catalog.md` (new — every metric enumerated).

## 2026-09-04 — Phase 1 Task 2: webhook ingress

Provider-agnostic HTTP intake for every future provider. Meta calls `POST /webhooks/whatsapp/{integration_id}` with a signed body; we verify, persist for idempotency, ACK 200 in single-digit milliseconds, and enqueue the raw body onto the `webhook.process` lane for the async worker.

- **Phase 1 Task 2 — webhook ingress**
- `internal/webhook/verifier.go` — `SignatureVerifier` interface + `SignatureVerifierFunc` adapter + `StaticVerifierLookup`. Adapters plug in without importing the webhook package; cmd/server wraps the existing `whatsapp.VerifySignature` free function.
- `internal/webhook/ingress.go` — `Ingress` pipeline: `MaxBytesReader(1 MiB)` → integration + secrets lookup (`org_id` derived from the row, never from the URL) → per-provider signature verification (before any parsing) → `sha256(raw_body)` as `external_event_id` → `webhook_events` insert (duplicate = 200 ACK) → enqueue on the `webhook.process` lane → 200 with empty body. Also exposes `VerifyHandshake` for Meta's `GET` subscription verification (`hub.mode` / `hub.verify_token` / `hub.challenge`).
- `internal/api/rest/v1/webhooks.go` + `router.go` — mounts `GET` + `POST /webhooks/{provider}/{integration_id}` outside `/api/v1/*` with a `RequestID → Recover → Logger` middleware chain only (no `SessionAuth`, no `RequireCSRF` — external providers don't carry our cookies).
- `internal/api/openapi/openapi.yaml` — bumped to `0.2.0-phase1` with both endpoints spec'd. Body cap surfaces as `413`; signature failure as `401`; provider/integration mismatch as `404`. All errors are RFC 7807 problem+json.
- `docs/flows/webhook-ingestion.md` — Mermaid rewritten to reflect the concrete pipeline (sig verify via `SignatureVerifier` → `webhook_events` insert → Kafka enqueue → ACK; separate handshake path).

## 2026-09-04 — Phase 1 Task 1: Kafka event log + job queue wiring

Replaces the Redis Streams placeholder from ADR 0004 with a real Kafka backend behind the existing `queue.Enqueuer` / `queue.Consumer` / `eventbus.Publisher` / `eventbus.Subscriber` ports. Redis stays for cache, locks, rate limits, idempotency, and presence.

- New `internal/infrastructure/kafka/` package built on `github.com/twmb/franz-go`:
  - `NewClient` / `NewAdmin` / `Close` — franz-go + kadm client construction with idempotent, snappy-compressed, `acks=all` producer defaults.
  - `EnsureTopics` — idempotent topic creation via kadm.
  - `NewProducer` — implements `queue.Enqueuer` (topic `<prefix>.jobs.<lane>`, partition key = `Job.ID`) and `eventbus.Publisher` (topic `<prefix>.events.<type>`, partition key = `Envelope.CorrelationID`) so per-conversation ordering falls out of the partitioner.
  - `NewConsumer` — implements `queue.Consumer` with disabled auto-commit; records are committed only after the handler returns nil, redelivered on error.
  - `NewEventBus` — implements `eventbus.Subscriber` with one managed background goroutine per event type, `Close` blocks on the goroutine set.
  - JSON codec for `queue.Job` + `events.Envelope` (protobuf follow-up in ADR 0009).
  - `Probe` for `/readyz`.
- `internal/infrastructure/config/config.go` gains a `KafkaConfig` (`Brokers`, `ClientID`, `TopicsPrefix`, `ReplicationFactor`, `DefaultPartitions`) with YAML + `FULLWA_KAFKA_BROKERS` env override.
- `config/example.yaml` and `config/local.yaml` gain a `kafka:` section pointing at `127.0.0.1:9092`.
- New ADR `docs/adr/0009-kafka-for-event-log.md` supersedes the Redis-Streams portion of ADR 0004.
- `docs/adr/0004-event-bus.md` gains a "Superseded by 0009 for durable path" footer.
- `docs/flows/webhook-ingestion.md` sequence updated to show Kafka as the queue.
- `docs/phases/phase-1.md` moves Kafka wiring from pending to shipped.

## 2026-09-03 — Phase 0 closed, Phase 1 foundation laid, live on `origin/main`

### `3bc7132` — fix(auth): `/me` returns `email` + `display_name`

Frontend inbox Header crashed with "Cannot read properties of undefined (reading 'trim')" — the `/me` handler wasn't returning the fields the initials helper reads.

- Added `UserLookup` interface in `internal/api/rest/v1/auth.go`.
- Added `Users.GetProfile(userID) (email, displayName)` in `internal/infrastructure/mysql/users.go`.
- Extended `Me` response struct + OpenAPI schema.
- Wired the existing users repo into `AuthDeps.Users` in `cmd/server/main.go`.

### `a0d820c` — chore(web): stop emitting `.js` next to `.tsx`

The frontend `build` script was `tsc -b && vite build` and `tsc -b` emits by default. Emitted 22 `.js` twins into `web/src/**` on the previous commit — cleaned up.

- `web/tsconfig.json` → `noEmit: true`.
- `web/package.json` scripts → `tsc --noEmit && vite build` (Vite handles TS via esbuild).
- `.gitignore` → `web/src/**/*.js`, `tsconfig.tsbuildinfo`.
- 605 lines of accidental emissions removed from history.

### `c3ed4e2` — feat: Phase 0 Task 2/3 + Phase 1 domain — auth E2E, WhatsApp adapter, inbox UI

Three parallel agents landed in one atomic commit. See [`docs/phases/phase-0.md`](docs/phases/phase-0.md) and [`docs/phases/phase-1.md`](docs/phases/phase-1.md) for the full breakdown.

**Backend auth (Phase 0 Task 2/3)** — full login flow end-to-end.

- MySQL repos: users, web_sessions (SHA-256 opaque row key), rbac, orgs, bootstrap.
- argon2id + CSRF double-submit + session cookies wired into the request path.
- Middleware chain: RequestID → Recover → Logger → SessionAuth → RequireAuth → RequireCSRF.
- REST v1 auth handlers (`csrf`, `login`, `logout`, `me`) with RFC 7807 errors.
- CLI: `tenant create`, `user create --admin`, `migrate up|down|status`.
- Health probes: MySQL + Redis with per-probe results in `/readyz`.

**Phase 1 domain + WhatsApp adapter** — foundation for the inbox flow.

- Real domain types with state machines: contact, identity, session, conversation, message. Provider-neutral payload shapes.
- Repository ports for all Phase 1 entities.
- WhatsApp Cloud API adapter implementing `channel.Provider`: retrying Graph client, `SendMessage` covering text/media/template/location/reaction/interactive, `ParseWebhook` covering the full documented inbound surface + preserving `unknown` fallback, media download, template CRUD, `HealthCheck`, capability matrix, provider self-registration.
- Migration `20260903000002_phase1_domain`: 9 tables with idempotency uniques and the STORED GENERATED trick to enforce one-active-session-per-endpoint at the DB.
- Docs: 5 domain pages + `providers/whatsapp.md` + 3 flow docs with Mermaid sequences.

**Frontend (Phase 0 UI)** — walking-skeleton browser app.

- TanStack Router + Query.
- `/login` (redirects to `/inbox` when authed), `/inbox` (protected three-pane), `/settings/integrations` (protected).
- Fetch wrapper with `credentials:'include'`, auto CSRF header, RFC 7807 → typed `ApiError`.
- Auth hooks `useMe` / `useLogin` / `useLogout`.
- Emerald + slate palette; rounded-xl cards; focus-trapped modal; accessible aria-labels.
- Strict TypeScript typechecks clean; Vite build produces `dist/` at ~91 kB gzipped.

### `02b6551` — chore: Phase 0 Task 1 — foundations, skeleton, CLAUDE.md harness, docs

Repo bootstrap. Modular monolith → single Go binary; MySQL + Redis + HBase all local-native (no Docker); React/Vite frontend to be embedded via `//go:embed`.

- Full 17-section `CLAUDE.md` operating manual.
- Config-first local infra via `config/local.yaml` + `scripts/check-infra.sh`.
- Makefile: check-infra, verify, gen, migrate, dev, build, coverage-check.
- Directory tree locked to spec §52 with `.go-arch-lint.yml` + grep guards enforcing the dependency direction.
- `cmd/server`: stdlib HTTP server booting `/healthz` `/readyz` `/metrics` with structured slog + graceful shutdown; smoke-tested end-to-end.
- `internal/domain/events`: 40+ canonical provider-agnostic event types.
- `internal/events`: in-proc fan-out event bus with fan-out + first-error tests.
- `internal/ports/{channel,ticketing,bot,aiport,calling,eventbus,queue,attachments,repository}`.
- `internal/providers/registry.go`: self-registering provider registry.
- `internal/infrastructure/config`: YAML + `FULLWA_*` env override loader.
- OpenAPI 3.1 skeleton with `Problem` schema (RFC 7807) + security schemes.
- First migration: organizations, users, teams, roles, permissions, user_roles, web_sessions, audit_logs (InnoDB / utf8mb4 / org-scoped indexes).
- Vite + React 18 + strict TS + Tailwind frontend scaffold.
- GitHub Actions CI: fmt, vet, golangci-lint, go-arch-lint, tests + coverage, Spectral OpenAPI lint, frontend typecheck + build.
- 8 ADRs: language, monolith, storage, event bus, auth, OpenAPI-first, testing, documentation.
- Living docs: architecture, onboarding, runbook, phase-0.

---

## Architecture decisions

See [`docs/adr/`](docs/adr/):

- `0001-language-and-deps.md` — Go + strict TS + minimal deps
- `0002-modular-monolith.md` — one binary, arch-lint enforcement
- `0003-storage-choices.md` — MySQL + Redis + HBase (native, no Docker)
- `0004-event-bus.md` — in-proc + Redis Streams behind one port
- `0005-auth-model.md` — session cookies + API keys, no JWT-in-JS
- `0006-openapi-first.md` — spec is the source of truth
- `0007-testing-strategy.md` — real DB integration tests; unit ≥80% domain
- `0008-documentation-strategy.md` — docs are a shipping artefact

## Delivery-workflow notes

- Three-agent parallelisation pattern used in commit `c3ed4e2`. Each agent gets a strictly non-overlapping file scope so they don't collide when writing concurrently to the same worktree. Integration pass by the driver rebuilds + re-runs tests to catch cross-agent interface drift.
- User preference: "working end-to-end first, tests later" — captured in Claude memory at `~/.claude/projects/-Users-senthil-11424-Documents-fullWA/memory/feedback_working_first_tests_later.md`.
- User preference: "no Docker / no Kubernetes anywhere" — captured at `~/.claude/projects/-Users-senthil-11424-Documents-fullWA/memory/feedback_no_docker_k8s.md`.
