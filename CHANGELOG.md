# Nudgeway CHANGELOG

Human-readable project history. Commit hashes are on `main` at `v-senthil/whatsapp-cloud-api`. For the OpenAPI-specific changelog see [`docs/api/CHANGELOG.md`](docs/api/CHANGELOG.md).

Format: reverse-chronological. Latest at the top.

---

## 2026-09-04 — Phase 3: WhatsApp Calling

End-to-end voice-calling slice lands: canonical `Call` domain, provider-agnostic `calling.Provider` port, WhatsApp Business Calling API adapter, MySQL persistence with idempotent webhook upserts, REST list/get/initiate/answer/reject/end/recording endpoints, canonical `call.*` events, and a top-level operator UI at `/calls`. The WhatsApp adapter now speaks `POST /<PHONE_NUMBER_ID>/calls` for connect/accept/reject/terminate and emits `Call*` events by parsing the `changes[].field=calls` webhooks (connect, call_created, terminate, call_recording_available, call_transcription_available, and RINGING/ACCEPTED/REJECTED status callbacks). `docs/providers/whatsapp.md` flips the Calls capability row to shipped in a follow-up wire-up commit.

- **Migrations `20260904000005_calls.up.sql` / `.down.sql`** — new `calls` table. VARBINARY(16) ULID PK; nullable linkage columns (`business_endpoint_id`, `contact_id`, `session_id`, `conversation_id`) so early-arriving webhooks persist even before reconciliation. UNIQUE `(org_id, provider, provider_call_id)` for idempotent upsert; indexes `(org_id, created_at DESC)`, `(org_id, status)`, `(org_id, contact_id, created_at DESC)`. Recording/transcription columns are lightweight refs; heavy bytes stream on demand.
- **Migration `20260904000005_grant_calls_perms.up.sql` / `.down.sql`** — idempotent `INSERT IGNORE` backfill granting `calls.read` + `calls.manage` to every admin role.
- **`internal/domain/call/{call.go,errors.go}`** — canonical `Call` struct + `Direction` (inbound/outbound) + `Status` state machine (queued → ringing → answered → in_progress → completed | missed | failed | declined | no_answer). Sentinel errors `ErrNotFound`, `ErrInvalid`, `ErrProviderUnsupported`.
- **`internal/ports/calling/calling.go`** — replaced the two-method stub with the full port: `InitiateCall(CallRequest) → CallResult`, `AnswerCall`, `RejectCall`, `EndCall`, `GetRecording` (returns `io.ReadCloser + content-type`), plus `Capabilities()` and `RecordingOptions` / `TranscriptionOptions` structs. New `Registry` interface for resolving adapters by `(providerKey, secrets)`.
- **`internal/ports/repository/calls.go`** — `CallRepo` with `Create`, `UpsertByProviderID`, `Get`, `List(filter)`, `UpdateStatus`, `AttachRecording`. `CallListFilter` supports direction, status, contact, since/until, cursor, limit.
- **`internal/application/call/service.go`** — `Service` orchestrates `RequestCall` (persist queued → invoke provider → stamp `provider_call_id` + status=ringing → publish `CallInitiated`), `ProcessInboundEvent` (webhook-driven upsert + backfill endpoint linkage), `Get`, `List`, `Answer` / `Reject` / `End` (dispatch to provider adapter + advance status). Defines local `CallingProviderRegistry` interface so the application never imports a concrete provider package. `GetRecording` proxies through the provider so the browser never sees a Meta short-lived URL.
- **`internal/infrastructure/mysql/calls.go`** — implements the port. `UpsertByProviderID` uses a two-step select-then-insert-or-merge with `COALESCE`-preserving semantics so a late-arriving webhook cannot blank a stamped timestamp. Opaque base64 cursor `(created_at_unix_micros | ULID)` for keyset pagination.
- **`internal/providers/whatsapp/calling.go`** (NEW) — `Provider` gains `InitiateCall` / `AnswerCall` / `RejectCall` / `EndCall` methods speaking Meta's `/<PHONE_NUMBER_ID>/calls` action vocabulary; `GetRecording` reuses the existing media-download two-step. Companion parser `ParseCallWebhook(rawBody, resolver)` handles `changes[].field=calls` envelopes (connect, call_created, terminate, RINGING/ACCEPTED/REJECTED statuses, call_recording_available, call_transcription_available). Capabilities disambiguation shim `CallingProvider()` returns a `calling.Provider` value whose `Capabilities()` returns `calling.Capabilities` (the underlying `*Provider.Capabilities()` still returns `channel.Capabilities` for the messaging port).
- **`internal/domain/events/call_events.go`** — new `CallInitiated` / `CallRinging` / `CallAnswered` / `CallEndedDetailed` / `CallFailed` `events.Type` constants + flat `CallEventPayload`. Coexists with the pre-existing `CallStarted` / `CallEnded` / `CallRecordingCreated` schema stability constants in `events.go`.
- **`internal/domain/rbac/perms_calls.go`** — new `PermCallsRead` + `PermCallsManage` constants. NOT added to `rbac.All()` — the migration backfill grants them to admin roles instead so we don't churn the initial seed catalogue.
- **`internal/api/rest/v1/calls.go`** — new `mountCalls` installing: `GET /api/v1/calls` (list + filters), `GET /api/v1/calls/{id}`, `POST /api/v1/calls` (initiate), `POST /api/v1/calls/{id}/answer|reject|end`, `GET /api/v1/calls/{id}/recording` (streams provider bytes). Read routes gated on `calls.read`; write routes require `calls.manage` + CSRF. Not yet wired in `router.go`; that's a follow-up commit.
- **`internal/api/openapi/fragments/calls.yaml`** (NEW) — self-contained OpenAPI 3.1 fragment for the seven endpoints + `Call` / `CallList` / `InitiateCallRequest` / `InitiateCallAccepted` / `RecordingOptions` / `TranscriptionOptions` schemas. Merged into `openapi.yaml` in a follow-up commit.
- **Frontend `web/src/lib/calls.ts`** — TanStack Query hooks: `useCalls` (infinite list), `useCall(id)`, `useInitiateCall`, `useAnswerCall`, `useRejectCall`, `useEndCall`. Types mirror the backend DTO.
- **Frontend `web/src/routes/calls.tsx`** — new top-level `/calls` route (not under settings) — a call log page with a filterable list on the left, a drill-down panel on the right, and inline `<audio controls>` playback of the recording served via the proxy at `/api/v1/calls/{id}/recording`.
- **Docs** — `docs/domain/call.md` (entity + state machine + invariants + code refs), `docs/flows/inbound-call.md` (mermaid), `docs/flows/outbound-call.md` (mermaid).

Wire-up (cmd/server: instantiate `mysql.NewCalls`, `application/call.New`, register the WhatsApp `CallingProvider()` in the Calling `Registry`, thread `CallsDeps` into `router.Mount`; add `/calls` route in `web/src/router.tsx`; flip the `Calls` row in `docs/providers/whatsapp.md`) is a follow-up commit — the new REST routes are silently omitted when `CallsDeps.Service` is nil so booting without them stays safe.

---

## 2026-09-04 — Phase 2: WhatsApp Groups

WhatsApp Groups land as a full vertical slice — domain, port, application service, infra, provider adapter methods, REST, RBAC, docs, and a frontend viewer with a send-to-group composer. The capability matrix in `docs/providers/whatsapp.md` can flip its "Groups" cell from ❌ to ✅ (docs update is a follow-up so the provider matrix file, which is on this task's deny list, stays untouched).

- **Migration `20260904000004_groups`** — new `groups` table (ULID `id`, `org_id`, `integration_id`, `provider_group_id`, `subject`, `description`, `size`, `is_admin`, `metadata` JSON, timestamps) with `UNIQUE (org_id, integration_id, provider_group_id)` mirroring Meta's per-integration group id uniqueness. New `group_members` table with BIGINT AUTO_INCREMENT PK, `(group_id, wa_id, bsuid)` unique key, nullable `contact_id` (populated lazily by the inbound resolver), and soft-delete `left_at`.
- **Migration `20260904000004_grant_groups_perms`** — idempotent `INSERT IGNORE` that backfills `groups.read` + `groups.manage` onto every existing admin role.
- **`internal/domain/group/{group.go,errors.go}`** — canonical `Group` + `Member` aggregates. `Role` type with `RoleMember` / `RoleAdmin` / `RoleSuperAdmin` constants; sentinel errors `ErrNotFound` / `ErrIntegrationMissing` / `ErrInvalidRole`; `ValidRole` guard. Zero infra + zero provider imports.
- **`internal/domain/rbac/perms_groups.go`** — new `PermGroupsRead` + `PermGroupsManage` in a dedicated file so the seed `All()` catalogue stays untouched; the migration handles the backfill.
- **`internal/domain/providercall/ops_groups.go`** — `OpListGroups` / `OpGetGroup` / `OpListGroupMembers` operation constants so the Meta API logs filter can enumerate them without touching `entry.go`.
- **`internal/ports/repository/groups.go`** — `GroupRepo` port with `Upsert` / `Get` / `GetByProviderID` / `List(with GroupListFilter)` / `Delete` / `AddMember` / `RemoveMember` / `ListMembers`. Every method is org-scoped; List paging uses the same opaque ULID cursor pattern as contacts + audit.
- **`internal/application/group/service.go`** — provider-agnostic `Service` with `Sync(orgID, integrationID)` (fetches groups + roster from the provider and upserts), `List` / `Get` / `Members`, and `SendMessage` (delegates through an injected `SendService`). Resolves the channel adapter via a `ProviderRegistry` shaped identically to the one used by `message.SendService`, then type-asserts to a small `ProviderGroupsClient` interface so channels that don't model groups can return `ErrUnsupported` cleanly.
- **`internal/providers/whatsapp/groups.go`** — new `Provider.ListGroups` / `GetGroup` / `ListGroupMembers` methods routed through the existing `client.doJSON` so tracer events fire automatically. Meta wire types kept private to the package; the exported `GroupSummary` / `GroupDetail` / `GroupMember` shapes structurally satisfy `application/group.ProviderGroupsClient` without any cross-package coupling.
- **`internal/providers/whatsapp/groups_inbound.go`** — additive helper carrying `MessageReceivedGroupPayload` + `ExtractGroupID`. `mapper.go` is on this task's deny list; a follow-up wire-up commit threads the group id into the canonical `MessageReceivedPayload` (marked with a `TODO(wire-up):` note).
- **`internal/infrastructure/mysql/groups.go`** — `Groups` struct implementing `GroupRepo`. Upsert resolves the target id via `(org, integration, provider_group_id)` first so the returned `Group.ID` is stable across re-syncs. `AddMember` upserts on `(group_id, wa_id, bsuid)` and clears `left_at` on rejoin. `Delete` cascades to members explicitly (no ON DELETE CASCADE — kept explicit for grep-ability).
- **`internal/api/rest/v1/groups.go`** — exports `MountGroups(mux, base, authed, deps)` so `cmd/server` can wire this feature without editing `router.go` (which is on this task's deny list). Routes: `GET /api/v1/groups` (auth + `groups.read`), `POST /api/v1/groups/sync` (auth + CSRF + `groups.manage`), `GET /api/v1/groups/{id}` (auth + `groups.read`), `GET /api/v1/groups/{id}/members` (auth + `groups.read`), `POST /api/v1/groups/{id}/messages` (auth + CSRF + `messages.send`). Sync failures classify: `424` missing integration, `422` unsupported provider, `502` provider transport.
- **OpenAPI** — self-contained fragment at `internal/api/openapi/fragments/groups.yaml` with schemas (`Group`, `GroupList`, `GroupMember`, `GroupMemberList`, `SyncGroupsRequest`, `SyncGroupsResponse`, `SendGroupMessageRequest`, `SendGroupMessageAccepted`), tag `groups`, and all five paths. Merge into the aggregate spec is a follow-up codegen step.
- **Frontend** — `web/src/lib/groups.ts` (TanStack Query hooks: `useGroups`, `useGroup`, `useGroupMembers`, `useSyncGroups`, `useSendGroupMessage`) and `web/src/routes/settings.groups.tsx` (list ↔ detail split-pane with member roster, role badges, integration + subject filter chips, one-click sync button, and a text-only send composer). Route exported as `settingsGroupsRoute` for the sidebar wire-up commit to pick up.
- **Docs** — `docs/domain/group.md` (purpose, invariants, code refs, wire-up notes) and `docs/flows/group-sync.md` (ASCII sequence, failure semantics, idempotency, observability).

Wire-up (`cmd/server/main.go` — construct `application/group.Service`, thread into `v1.MountGroups(...)`; `web/src/router.tsx` — add `settingsGroupsRoute` to the tree; `web/src/routes/settings.tsx` — add a nav link) is a follow-up commit: every REST route is silently omitted when `GroupsDeps.Service` is nil, so booting without the wire-up stays safe.

---

## 2026-09-04 — Phase 2: Template management

WhatsApp templates land as a full vertical slice — domain, port, application service, infra, provider adapter, REST, RBAC, docs, and frontend. The capability matrix in `docs/providers/whatsapp.md` finally has code to back the ✅ next to "Templates".

- **Migration `20260904000003_templates`** — new `templates` table with the ULID PK / VARBINARY org_id shape every other Phase 1/2 table uses. Unique key on `(org_id, integration_id, name, language)` mirrors Meta's per-WABA uniqueness invariant. `components` + `variables` land as JSON for schema stability; `last_synced_at` NULL means "never reconciled".
- **Migration `20260904000004_grant_templates_perms`** — idempotent `INSERT IGNORE` that backfills `templates.read` + `templates.manage` onto every existing admin role.
- **`internal/domain/template/{template.go,errors.go}`** — canonical `Template` entity, `Category` + `Status` string types, `Component` union struct, sentinel errors (`ErrNotFound` / `ErrInvalid` / `ErrIntegrationMissing` / `ErrNotSubmittable`). Zero infra imports; zero provider imports.
- **`internal/domain/rbac/perms_templates.go`** — `PermTemplatesRead` + `PermTemplatesManage` constants in their own file so the base `rbac.go` stays untouched.
- **`internal/ports/repository/templates.go`** — `TemplateRepo` interface (`Create`, `Get`, `List`, `Upsert`, `UpdateStatus`, `Delete`) + `TemplateListFilter` + `TemplatePage`.
- **`internal/application/template/service.go`** — `Service` with `Create` (DRAFT + optional submit), `SubmitForReview`, `Sync` (fetch every provider row + upsert), `Get`, `List`, `Delete`. Defines the narrow `TemplateProvider` + `ProviderRegistry` ports; never imports a concrete provider. Validation rejects malformed names, unknown categories, missing BODY components.
- **`internal/infrastructure/mysql/templates.go`** — `Templates` repo with opaque base64 `(updated_at | id)` cursor pagination, `ON DUPLICATE KEY UPDATE` upsert on the natural key, ULID + JSON marshalling shared with the sibling repos.
- **`internal/providers/whatsapp/templates.go`** — extended the stub with typed `ListTemplates` / `CreateTemplate` / `GetTemplateStatus` methods. `TemplateSummary`, `TemplateCreateRequest`, `TemplateStatus` are provider-native; the application boundary maps them into `template.Template`. Raw JSON variants (`ListTemplatesRaw`, ...) stay for operator tooling. Existing client-level `listTemplates` / `createTemplate` / `getTemplateStatus` already have tracer hooks so execution-log persistence is automatic.
- **`internal/api/rest/v1/templates.go`** — six handlers behind auth + RBAC (`templates.read` on GETs, `templates.manage` + CSRF on writes): `GET/POST /api/v1/templates`, `GET/DELETE /api/v1/templates/{id}`, `POST /api/v1/templates/{id}/submit`, `POST /api/v1/templates/sync`. RFC 7807 problem responses throughout. Route silently omitted when `TemplateDeps.Service` is nil.
- **OpenAPI** — self-contained fragment at `internal/api/openapi/fragments/templates.yaml` declaring every path + `Template` / `TemplateComponent` / `TemplateStatus` / `TemplateCategory` schemas. Spliced into `openapi.yaml` on the next `make gen-api` pass.
- **Frontend** — new `web/src/lib/templates.ts` with TanStack Query hooks (`useTemplates`, `useTemplate`, `useCreateTemplate`, `useSubmitTemplate`, `useSyncTemplates`, `useDeleteTemplate`). New `web/src/routes/settings.templates.tsx` renders a list + status badge column + a lightweight new-template wizard (name / language / category / body). Sync button pulls from the provider on demand.
- **Docs** — new `docs/domain/template.md` (entity purpose + invariants + lifecycle diagram) and `docs/flows/template-sync.md` (sequence + retry semantics). Wire-up (`cmd/server/main.go`, `router.go`, `router.tsx`, `settings.tsx` sidebar) is a follow-up commit.

---

## 2026-09-04 — Phase 2: Analytics v1

Daily rollups over the canonical `messages` and `conversations` tables now feed an operator dashboard. Zero coupling to the WhatsApp adapter — the rollup pipeline and read endpoints are provider-agnostic and reference providers only as opaque strings. Cards show messages total, delivery-rate percentage, coarse p50 response time, and conversations opened; two sparkline charts plot messages/day and delivery-rate over time.

- **Migration `20260904000006_analytics_rollups`** — three rollup tables (`analytics_messages_daily`, `analytics_conversations_daily`, `analytics_delivery_rate_daily`) all keyed on composite PKs so upserts are safe, plus `analytics_rollup_state` bookmark. Sentinel `provider='all'` + `message_type='all'` rows carry pan-provider / pan-type grand totals so dashboard cards don't need a runtime SUM().
- **Migration `20260904000007_grant_analytics_perms`** — idempotent `INSERT IGNORE` that backfills `analytics.read` onto every existing admin role.
- **`internal/domain/analytics/analytics.go` + `errors.go`** — canonical typed shapes (`MessagesDaily`, `ConversationsDaily`, `DeliveryRateDaily`, `Point`, `Series`, `Overview`) plus `SeriesKind` enum and `ErrInvalidRange` / `ErrUnknownSeries` / `ErrInvalidRollupDay`. Zero infra imports.
- **`internal/domain/rbac/perms_analytics.go`** — new `PermAnalyticsRead` constant. Lives in its own file so the base `rbac.go` `All()` list stays untouched (grant flows through the migration).
- **`internal/ports/repository/analytics.go`** — `AnalyticsRepo` with three upsert methods, three range readers, and `RollupState` / `SaveRollupState` for the worker bookmark.
- **`internal/ports/repository/analytics_source.go`** — new `AnalyticsSource` port: `CountMessagesByDay`, `CountConversationsByDay`, `P50ResponseTimeByDay`. Keeps raw-SQL access to `messages` + `conversations` behind an interface so tests can substitute an in-memory fake.
- **`internal/application/analytics/service.go`** — `Service.Rollup(ctx, orgID, day)` folds raw per-(provider, direction, type) counts into detail rows AND pan-provider / pan-type aggregate rows, upserts three tables. `Overview` reads pan-provider aggregate rows and computes the delivery-rate ratio + a coarse p50 over per-day averages. `Series` returns time-series for the three supported kinds with an optional per-provider filter. Every write is idempotent.
- **`internal/infrastructure/mysql/analytics.go`** — `Analytics` repo implementing `AnalyticsRepo` with batched `ON DUPLICATE KEY UPDATE` inserts. Range readers scope every query by `org_id` and `BETWEEN` on `day`.
- **`internal/infrastructure/mysql/analytics_source.go`** — `AnalyticsSource` implementation. `CountMessagesByDay` uses a single `GROUP BY provider, direction, message_type` with `SUM(CASE ...)` counters. `P50ResponseTimeByDay` self-joins `messages` with a `NOT EXISTS` clause to pair each inbound message with the *next* outbound on the same conversation, capped at day+24h.
- **`internal/workers/analytics_rollup.go`** — `AnalyticsRollupRunner` with an injected `OrgLister` port. Ticks every 15 minutes (immediate first tick on start), rolls up yesterday + today for every org, persists the bookmark. One goroutine per runner — no unbounded fan-out. Tenant errors are logged and swallowed so a broken org doesn't block the rest.
- **`internal/api/rest/v1/analytics.go`** — new `GET /api/v1/analytics/overview` and `GET /api/v1/analytics/series` behind auth + `analytics.read`. Date range via `?from=&to=` (YYYY-MM-DD, UTC). `series` accepts `?kind=messages_daily|delivery_rate|conversations_opened` and an optional `?provider=`. Routes silently omitted when `AnalyticsDeps.Service` is nil so slim deploys stay safe. Wire-up (`cmd/server/main.go`, `router.go`) is a follow-up commit — no files owned by other agents are modified.
- **OpenAPI** — self-contained fragment at `internal/api/openapi/fragments/analytics.yaml` declaring the two endpoints, the `analytics` tag, and `AnalyticsOverview` / `AnalyticsSeries` / `AnalyticsPoint` schemas. Spliced into `openapi.yaml` on the next `make gen-api` pass.
- **Frontend** — new `web/src/lib/analytics.ts` with TanStack Query hooks (`useAnalyticsOverview`, `useAnalyticsSeries`). New `web/src/routes/analytics.tsx` renders four KPI cards + two SVG sparklines. No charting library added — inline SVG keeps the bundle unchanged.
- **Docs** — new `docs/domain/analytics.md` (rollup semantics + backfill note) and `docs/flows/analytics-rollup.md` (sequence + idempotency guarantees).

---

## 2026-09-04 — Meta API execution logs

Every outbound HTTP call the WhatsApp adapter makes to Meta is now recorded to MySQL for operator debugging. The only diagnostic previously was a `getDebugSink()` fprintf on 4xx/5xx responses — that stays, but each request/response now also lands in `provider_calls` with request body, response body, status code, latency, error class, and Meta's `fbtrace_id`. A new `GET /api/v1/provider-calls` endpoint powers the settings viewer.

- **Migration `20260904000001_provider_calls`** — new `provider_calls` table (BIGINT AUTO_INCREMENT PK, VARBINARY(16) `org_id` + nullable `integration_id`, MEDIUMBLOB request/response bodies, `status_code`, `latency_ms`, `error_class`, `error_message`, `trace_id`, `correlation_id`, `occurred_at`). Three composite indexes: `(org_id, occurred_at)`, `(org_id, integration_id, occurred_at)`, `(org_id, status_code)`.
- **`internal/domain/providercall/entry.go` + `errors.go`** — new domain entity + `ErrNotFound`. `Entry.Redact()` is a no-op today (kept for future header capture). Constants `Op*` enumerate the WhatsApp operations. `Direction` is `outbound` today; `inbound` reserved for a future refactor consolidating webhook_events onto this same table.
- **`internal/ports/repository/provider_call.go`** — `ProviderCallRepo` with `Record` + `List` and a `ProviderCallListFilter` (integration, operation, status range, since/until, cursor, limit). Cursor is opaque base64 of `<occurred_at_unix_micros>|<id>`.
- **`internal/application/providercall/service.go`** — `Service.Record` is fire-and-forget: persistence errors are logged and swallowed so a downed MySQL can never break the outbound send path. Request / response bodies are truncated to `MaxBodyBytes` (default 64 KiB) before persist. `Service.List` proxies to the repo.
- **`internal/infrastructure/mysql/provider_calls.go`** — `ProviderCalls` struct implementing the port. Follows the audit-log pattern (same cursor style, same scan helper shape).
- **`internal/providers/whatsapp/tracer.go`** — new `Tracer` interface + `TraceEvent` struct + `NopTracer` default. Tracers must return quickly and must not block the caller — the persistence contract enforces fire-and-forget.
- **`internal/providers/whatsapp/config.go`** — `Config` gains `Tracer`, `IntegrationID`, `OrgID`. Nil `Tracer` transparently falls back to `NopTracer` so unit tests never crash.
- **`internal/providers/whatsapp/client.go`** — `doJSON` / `doOnce` now take an `op string` parameter and emit exactly one `TraceEvent` per attempt (both success and failure). `downloadMedia` records latency + status but never the response body (raw bytes are the media itself). Every 4xx/5xx path also stamps Meta's `fbtrace_id`.
- **`internal/providers/whatsapp/upload.go`** — `uploadMedia` emits a `TraceEvent` with a synthetic request body (`{filename, content_type, size}`) rather than the raw multipart bytes.
- **`internal/providers/whatsapp/provider.go`** — new `Provider.WithTracer(t, integrationID, orgID)` fluent setter so `cmd/server` can wire the tracer alongside `WithEndpointResolver`. Tracing stays provider-internal — the `channel.Provider` port never grows a bookkeeping method.
- **`internal/api/rest/v1/provider_calls.go`** — new `GET /api/v1/provider-calls` handler behind `integrations.manage`. Query params: `integration_id`, `operation`, `status_min`, `status_max`, `since` / `until` (RFC 3339), `cursor`, `limit` (1..200, default 50). Response items carry both base64 and best-effort UTF-8 renderings of request / response bodies so the UI can pretty-print JSON without a second round-trip.
- **OpenAPI** — shared `0.2.4-phase2` block with the parallel audit-log agent. New `GET /api/v1/provider-calls`, schemas `ProviderCall` + `ProviderCallList`, tag `provider-calls`.
- **Frontend** — new `web/src/routes/settings/provider-calls.tsx` table view with columns Time / Operation / Method / URL / Status / Latency / Error. Row-expand reveals the full request + response bodies. Filter chips: integration select, operation dropdown, status-code preset (all / 2xx / 4xx / 5xx), since / until inputs. Nav link exported via `web/src/routes/settings/_nav-provider-calls.ts` so the sidebar can import it without touching this file directly.
- **Docs** — new `docs/domain/provider_call.md` (purpose, invariants, retention note), new `docs/flows/provider-call-recording.md` (mermaid: request → doOnce → tracer.OnCall → application/providercall.Record → MySQL). `docs/providers/whatsapp.md` gains an "Observability — Meta API execution logs" section.

Wire-up (`cmd/server/main.go` — instantiate `providercall.Service`, thread into REST `Deps`, chain `WithTracer` on the WhatsApp Provider construction) is a follow-up commit; the new REST route is silently omitted when `ProviderCallsDeps.Service` is nil, so booting without it stays safe.

---

## 2026-09-04 — Audit logs (Phase 2)

Append-only audit trail lands as a full vertical slice: domain entity, repository port, MySQL implementation (uses the `audit_logs` table seeded in `20260903000001`), application service that swallows write failures so audit hiccups can never break the caller's mutation, and a paginated REST list at `GET /api/v1/audit-logs` gated on the new `audit.read` permission. Nine canonical action verbs are defined for the initial mutation set (integration lifecycle, message send/read, attachment upload, login/logout). Frontend ships a filterable trail view under `/settings/audit`.

- **`internal/domain/audit/*`** — `Entry` struct + nine `Action` constants + sentinel errors.
- **`internal/ports/repository/audit.go`** — `AuditRepo` interface (`Record`, `List`) with `AuditListFilter`.
- **`internal/application/audit/service.go`** — `Service.Record` (fire-and-forget, logs on failure) and `Service.List`.
- **`internal/infrastructure/mysql/audit.go`** — cursor-based pagination on `(occurred_at, id)`, base64-opaque cursor tokens.
- **`internal/api/rest/v1/audit.go`** — `GET /api/v1/audit-logs` with RFC 7807 error handling.
- **`internal/api/openapi/openapi.yaml`** — `AuditLog`, `AuditLogList`, `AuditLogFilter` schemas; `audit` tag; version bumped to `0.2.4-phase2`.
- **`internal/domain/rbac/rbac.go`** — new `PermAuditRead` (`audit.read`) added to `All()` so admin roles inherit it.
- **`web/src/routes/settings/audit.tsx`** + `web/src/lib/audit.ts` — TanStack Query hook, cursor-based Load More, filter chips (action, resource type, actor id, since/until date pickers). Sidebar link added in `settings.tsx`.
- **Not wired yet:** `Service.Record` is not threaded into existing mutation services in this commit — that lands with the wire-up + instrumentation follow-up. The read surface is fully functional against seeded rows today.

---

## 2026-09-04 — Phase 1 CLOSE: attachments to HBase, BSUID, live ticks, settings entry

Phase 1 (WhatsApp Inbox MVP) is functionally complete end-to-end.

- **`ed22d4f`** — feat(header): settings gear icon next to the profile avatar; clicking opens `/settings/integrations`.
- **`6195ea4`** — fix(send): persist `provider_message_id` (wamid) after each outbound send via new `mysql.Messages.SetProviderMessageID`. Without it, Meta's `delivered` / `read` status callbacks referenced a wamid with no matching DB row and the ticks never advanced past single-grey. Also reverts the BSUID-first send preference to phone-first with BSUID fallback (Meta's portfolio-side BSUID send is still rolling out — phone is universally accepted today; TODO tracked).
- **`16e0665`** — feat(bsuid): switch to Meta business-scoped user id (BSUID) as the primary identity. New mapper fields for `user_id`, `parent_user_id`, `username`, `from_user_id`, `from_parent_user_id`, `recipient_user_id`. `MessageReceivedPayload` gains `FromUserID` / `FromParentUserID` / `FromUsername`; `MessageStatusPayload` gains `RecipientUserID`. `InboundService` upserts a `bsuid` identity bound to the same Contact and promotes it to primary. See `~/Documents/whatsapp_doc_tracker/docs/business-scoped-user-ids.md`.
- **`60ebdf5`** — fix(send): media message flow. Frontend sends `media_id` alongside `url`; `SendService.RequestSend` stashes `media_url`, `media_id`, `caption`, `filename` into `metadata` for the outbound row so bubbles render immediately.
- **`6030038`** — feat(attachments): Meta Media Upload wired end-to-end. `POST /api/v1/attachments` stores bytes to HBase AND uploads to Meta's `POST /{phone_number_id}/media` via the WhatsApp adapter; returned `media_id` is stashed on the HBase row (`m:media_id_<provider>_<integration>`). Response now `{attachment_id, media_url, media_id, provider, size, content_type, filename}`. Composer prefers `media_id` (Meta-native handle) with `media_url` as fallback.
- **`95ce274`** — feat(attachments): HBase-backed store. New `internal/infrastructure/hbase/{client,schema,attachments}.go` (gohbase). Table `nudgeway_attachments`, column families `d` (data) + `m` (metadata). Row key = SHA-256 hex. Falls back to `LocalFS` when HBase unreachable.
- **`12c6871`** — fix: (a) media download uses the URL Meta ships in the webhook directly (one HTTPS round-trip), (b) `MessageReceivedPayload` gains `ConversationID` set before publish so WS bridge routes to `['messages', <id>]`, (c) reactions render only near their target — the orphan-reaction-bubble at top is gone.
- **`0394203`** — feat(phase1): media in/out + status ticks live + mark-as-read + rich rendering (location / contacts / interactive / reactions). Four parallel agents.
- **`40e4a32`** — fix(send): outbound to Meta uses real `phone_number_id` from `integration.Config` (was empty → double-slash URL) and the E.164 phone as `to` (was leaking internal ULID).
- **`adc9121`** — fix(pub): `combinedPublisher.Publish` fires Kafka in a detached goroutine so REST send returns in ~40 ms instead of 22 s.
- **`7e87965`** — fix(inbox): WhatsApp-style thread — oldest at top, newest at bottom.
- **`fd8f908`** — fix(messages): accept `text` as bare JSON string or canonical `{"body": "..."}` object.
- **`571cb39`** — feat(webhook): dev-mode fallback — payload-claims match instead of HMAC. `RequireSignature bool` gate (set `NUDGEWAY_REQUIRE_SIGNATURE=0` for dev). Boot logs a WARN when disabled.
- **`f6fc1e5`** — fix(integrations): `disconnected` surfaces as "Not connected" (grey) instead of "Unknown".
- **`1c90b67`** — chore(webhook): log `body_len` / `secret_len` / sig-prefix on HMAC mismatch for debugging.
- **`be4f65e`** — fix(integrations): REST Create now envelope-encrypts secrets to `integration_credentials` (was silently dropping them, so Meta's callback-verify 403'd).
- **`aaa5e1d`** — fix(web): `useCreateIntegration` splits flat form input into `{type, provider, name, config, secrets}`.
- **`1240b34`, `98a925d`, `b24cdba`** — inbox listing polish: conversation-list SQL joins for real `contact_name` + `last_message_preview` + `last_message_at`; frontend list shape uses `{items}` everywhere.
- **`517af31`** — fix(phase1): six integration fixes so the inbound webhook pipeline works end-to-end.
- **`8a2207c`** — feat(server): full Phase 1 wire-up. MySQL Phase 1 repos + envelope crypto + Kafka + worker pools + WebSocket hub + webhook ingress.

---

## 2026-09-04 — Phase 1 correctness: status ticks + mark-as-read

Live status callbacks now propagate to the browser without a refresh, and operators can push the "read" signal (blue double-tick) back to the customer's phone. Two REST endpoints ship the mark-as-read pipeline: `POST /api/v1/messages/{id}/read` for a single message and `POST /api/v1/conversations/{id}/read` for a batch capped at 50 unread inbound rows. The frontend auto-fires the batch variant when the operator opens a conversation with unread inbound, throttled to once per 5 s per conversation.

- **`internal/ports/channel/channel.go`** — extended `channel.Provider` with `MarkAsRead(ctx, providerMessageID) error`. Non-supporting adapters return `nil`; the WhatsApp adapter is the sole implementation today.
- **`internal/providers/whatsapp/client.go` + `provider.go`** — `client.markAsRead` POSTs `{messaging_product:"whatsapp", status:"read", message_id:<wamid>}` to `/{phone_number_id}/messages`. `Provider.MarkAsRead` guards on empty wamid, flips `healthy=false` on auth-class errors, otherwise reuses the shared retrying `doJSON` core.
- **`internal/application/message/read.go`** — new `ReadService` with `MarkRead(ctx, orgID, messageID)` and `MarkConversationRead(ctx, orgID, convID, cap)`. Both resolve conversation → session → endpoint → integration → provider adapter, call `provider.MarkAsRead`, and stamp `read_at` locally (Meta does not deliver a status callback for business-side reads). Idempotent — outbound / no-wamid / already-read messages are silently skipped.
- **`internal/ports/repository/message_repo.go` + `internal/infrastructure/mysql/messages.go`** — `MessageRepo.Get(ctx, orgID, id)` added; the MySQL impl selects on `(org_id, id)` with the shared `messageCols` list.
- **`internal/api/rest/v1/messages.go` + `router.go`** — new `MessagesDeps.Read`; `POST /messages/{id}/read` and `POST /conversations/{id}/read` mount when non-nil. Both auth + CSRF, return 204 on success, 404 / 424 / 502 on domain / integration / provider errors.
- **`cmd/server/main.go`** — wires `NewReadService(ReadDeps{...})` and passes it through `MessagesDeps.Read`.
- **`web/src/features/inbox/renderers/TickIcon.tsx`** — placeholder text-glyph component replaced with SVG double-ticks (grey delivered / sky-blue read), single-tick (grey sent), three-dot (queued/sending), and a red exclamation for failed. Uses `currentColor` so bubbles can tint the icon.
- **`web/src/lib/ws.ts` + `events.ts`** — status envelopes (`message.delivered`, `message.read`, `message.failed`) now invalidate the TanStack Query cache; when the WebSocket payload does not carry a `conversation_id` (the domain `MessageStatusPayload` is keyed on wamid) the hook invalidates every `['messages']` key so the tick flips without a refresh.
- **`web/src/lib/messages.ts`** — `useMarkMessageRead()` + `useMarkConversationRead()` mutations targeting the new endpoints.
- **`web/src/features/inbox/Thread.tsx`** — minimal effect that fires `useMarkConversationRead` on conversation-change / message-load when there is at least one unread inbound row, throttled to once per 5 s per conversation via a `useRef` timestamp.
- **OpenAPI** — bumped to `0.2.3-phase1`; adds `postMessageMarkRead` and `postConversationMarkRead`.
- **Docs** — `docs/flows/inbound-message.md` gains a mark-as-read sequence diagram; `docs/providers/whatsapp.md` documents the `MarkAsRead` port surface, Meta's 30-day / 131009 constraint, and the wire path.

TODO: retry semantics on `Provider.MarkAsRead` are still permanent-fail — the mutation returns 502 on the first Meta 5xx and the operator is expected to refire. Follow-up work: enqueue a "mark-read" side-lane on the Kafka bus and let the send worker retry with backoff.

---

## 2026-09-04 — Phase 1 correctness: inbound media

- Inbound WhatsApp media (image / video / audio / document / sticker) is now downloaded from Meta on receipt, stored in the content-addressed attachments store, and served through our own `GET /api/v1/media/{key}` — closing the gap where the browser previously rendered "[image] media message" because the raw bytes were never fetched.
- **`internal/infrastructure/attachments/localfs.go`** + **`store.go`** — new local-filesystem `attachments.Store` implementation for dev. `Put` streams into a temp file while computing SHA-256, then atomically renames onto `<root>/aa/bb/<digest>` and drops a `.contenttype` sidecar so `Get` (and the REST handler) can surface the MIME string without sniffing. `Get` / `Delete` refuse non-hex 64-char keys as a path-traversal guard.
- **`internal/infrastructure/config/config.go`** + **`config/example.yaml`** — new `attachments: root: "./attachments"` block; default is `./attachments` relative to the process working dir.
- **`internal/application/message/deps.go`** + **`inbound.go`** — `Deps` gains `Attachments attachments.Store` + `Downloader AttachmentDownloader` (both optional; nil pair emits a one-shot WARN and skips the media branch). `handleInbound` now, for any `MediaPayload` with a non-empty `MediaID`, calls `Downloader.Download(ctx, provider, integrationID, mediaID)` → `Attachments.Put` → stamps `attachment_key`, `content_type`, `file_size` on `message.metadata`. Download failures WARN-log and are swallowed so the message row still commits and the frontend shows a subtle "Attachment unavailable" fallback.
- **`internal/api/rest/v1/media.go`** + **`router.go`** — new `GET` / `HEAD /api/v1/media/{key}` route, auth-gated, streams from the store with `Cache-Control: private, max-age=86400`. Unknown keys return 404. Router gains `Attachments AttachmentsDeps { Store, Logger }`; nil `Store` omits the route.
- **`internal/api/rest/v1/messages.go`** — `MessageDTO` gains `ContentType`; `toMessageDTO` prefers `metadata.attachment_key` (surfacing `media_url: /api/v1/media/<key>`) over the short-lived provider-native URL.
- **`internal/providers/whatsapp/provider.go`** — new `Provider.DownloadMedia(ctx, mediaID) (io.ReadCloser, contentType, error)` that packages Meta's two-step `getMediaURL` + `downloadMedia` client helpers into a single streaming call. The previous `*Media` wrapper is preserved as `DownloadMediaMetadata` for callers who need SHA-256 / filesize alongside the stream.
- **`web/src/features/inbox/renderers/MediaBubble.tsx`** — real renderer replacing the placeholder: `<img>` for image (rounded, `max-w-xs`) with a skeleton loader, `<video controls>` for video, `<audio controls>` for audio, styled download link with filename + download icon for document, fixed 128×128 `<img>` for sticker, caption below where applicable, and "Attachment unavailable" broken-media fallback. Self-contained — the driver composes it in the bubble shell.
- **`web/src/lib/messages.ts`** — `Message.content_type?: string` added so the document renderer can surface the MIME string.
- **OpenAPI** — new `GET /api/v1/media/{key}` + `HEAD` operations returning `application/octet-stream`; `Message` schema gains `text`, `content_type`, and a documented `media_url`.
- **Docs** — `docs/providers/whatsapp.md` gains a "Media inbound flow" section; `docs/flows/inbound-message.md` extends the Mermaid diagram with the download / store branch and adds a "Media persistence" section.

Driver wire-up (`cmd/server/main.go`) is unchanged in this commit — the new deps are optional and default to nil, so booting Nudgeway without a media root is safe. The follow-up wire commit will instantiate `attachments.New(cfg.Attachments)`, close over the WhatsApp adapter as a `message.AttachmentDownloader`, and thread both into `appmsg.Deps` + `v1.Deps.Attachments`.

---

## 2026-09-04 — Phase 1 correctness: outbound media

- Operators can now attach a file to a WhatsApp send. `POST /api/v1/attachments` (auth + CSRF) accepts a `multipart/form-data` `file`, streams it through `attachments.Store`, and returns `{attachment_id, media_url, size, content_type, filename}`; the composer then calls `POST /api/v1/messages` with `type=image|video|audio|document` and `media: {url: <media_url>, caption?, filename?}`. The send worker hands the URL to the WhatsApp adapter's `canonicalSendToMeta`, which emits `{type:"image", image:{link:"…"}}` (or the corresponding video/audio/document/sticker shape) so Meta fetches + delivers.
- **`internal/api/rest/v1/attachments.go`** — new `POST /api/v1/attachments` handler. Enforces a 16 MiB cap via `http.MaxBytesReader` + `multipart.FileHeader.Size`, sniffs content-type when the multipart part omits it, and returns 201 with the fully-qualified `media_url` prefixed with `PublicBaseURL`. `AttachmentsUploadDeps` bundles `Store attachments.Store`, `PublicBaseURL string`, `Logger *slog.Logger`; a nil `Store` silently omits the route.
- **`internal/api/rest/v1/router.go`** — new `AttachmentsUpload AttachmentsUploadDeps` field on `Deps`; `mountAttachmentsUpload(mux, authed, deps.AttachmentsUpload)` wired after `mountMessages` (reuses the same auth + CSRF chain).
- **`web/src/lib/attachments.ts`** — `useUploadAttachment` mutation posts multipart with the standard CSRF header; `mediaKindFromContentType` maps a MIME type to the canonical `image|video|audio|document` message type; `MAX_ATTACHMENT_BYTES` mirrors the backend cap.
- **`web/src/features/inbox/renderers/ComposerAttach.tsx`** — paperclip button + hidden file input, thumbnail preview for images, filename + size preview otherwise, inline clear button.
- **`web/src/features/inbox/Composer.tsx`** — integrates `ComposerAttach`, kicks off upload on file select, blocks send while upload is in flight, sends either `text`, `media`, or `media + caption` in one request; optimistic row + rollback preserved.
- **`web/src/lib/messages.ts`** — `SendMessageInput` becomes a discriminated union (`text` / media). `useSendMessage` builds the correct backend body, echoes `client_reference_id` as `idempotency_key`.
- **OpenAPI** — bumped to `0.2.2-phase1`: new `POST /api/v1/attachments` path + `AttachmentUploadResponse` schema.
- **Docs** — `docs/providers/whatsapp.md` gains a "Media outbound flow" section (Meta resumable upload for > 16 MiB flagged as TODO). `docs/flows/outbound-send.md` Mermaid extended with the upload step + endpoint list documents the new route.

---

## 2026-09-04 — Phase 1 correctness: location/contacts/reactions/interactive rendering

- Inbound WhatsApp location, contacts, reactions, and interactive (list_reply / button_reply / template button) messages now render as proper bubbles in the operator inbox instead of the "[image] media message" placeholder. Reactions overlay as a small emoji chip on the reacted-to bubble (looked up by `provider_message_id`) and fall back to a standalone "Reacted <emoji>" bubble when the target is outside the loaded window.
- **Backend** — `internal/application/message/inbound.go` extends the metadata surfacing to flatten `LocationPayload`, `ContactsPayload`, `ReactionPayload`, and `InteractivePayload` into `message.Metadata` under stable keys (`location`, `contacts`, `reaction`, `interactive`), plus `reply_to_wamid` whenever the envelope carries a `context.id`. `internal/api/rest/v1/messages.go`'s `MessageDTO` gains optional `location`, `contacts`, `reaction`, `interactive`, and `reply_to_provider_message_id` fields (JSON tags with `omitempty`); `toMessageDTO` unmarshals them from metadata. OpenAPI `components.schemas.Message` documents all five new fields.
- **Frontend** — new `web/src/features/inbox/renderers/{LocationBubble,ContactCardBubble,ReactionBadge,InteractiveBubble,UnknownBubble}.tsx`. `Thread.tsx` is rebuilt around a `BubbleDispatch` component that switches on `msg.type`, with reactions pre-grouped by `groupReactions` so they overlay on their targets (or render as fallback bubbles). `web/src/lib/messages.ts` widens the `Message` type with the new optional fields.
- **Docs** — `docs/providers/whatsapp.md` gains a "DTO surface for specialised inbound types" table pairing each canonical payload with the DTO fields the frontend consumes.

---

## 2026-09-04 — Phase 1 Task 5: integrations API + CLI

Operators can list / create / test / disconnect provider integrations behind `/api/v1/integrations/*`, and a new `nudgeway-cli integration create` subcommand seeds a WhatsApp integration without touching the UI. Secret material is envelope-encrypted at rest and never crosses the API boundary — the response's `webhook_url` is the tenant-facing URL to paste into the provider console.

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
- `internal/infrastructure/metrics/metrics.go` — dedicated `*prometheus.Registry` plus the Nudgeway canonical metric families: HTTP requests + latency, provider calls + latency, worker jobs + latency + retries, webhook events received, Kafka producer batch bytes + consumer lag, WebSocket connections. Registers `GoCollector` + `ProcessCollector`. `Metrics.Handler()` serves the OpenMetrics exposition.
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
- `internal/infrastructure/config/config.go` gains a `KafkaConfig` (`Brokers`, `ClientID`, `TopicsPrefix`, `ReplicationFactor`, `DefaultPartitions`) with YAML + `NUDGEWAY_KAFKA_BROKERS` env override.
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
- `internal/infrastructure/config`: YAML + `NUDGEWAY_*` env override loader.
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
- User preference: "working end-to-end first, tests later" — captured in Claude memory at `~/.claude/projects/-Users-senthil-11424-Documents-Nudgeway/memory/feedback_working_first_tests_later.md`.
- User preference: "no Docker / no Kubernetes anywhere" — captured at `~/.claude/projects/-Users-senthil-11424-Documents-Nudgeway/memory/feedback_no_docker_k8s.md`.
