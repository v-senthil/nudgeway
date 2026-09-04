# Phase 2 — Multi-provider readiness + operator trust

Status: **in progress**. Phase 2 broadens fullWA from a WhatsApp inbox MVP into a multi-provider platform with the operator-trust surfaces (audit trail, per-provider call telemetry, richer RBAC) that make it safe to run in production.

## Goal (from the master plan)

Give tenant admins a durable, queryable answer to "what happened, who did it, when, and against which resource" — and give backend engineers a durable answer to "what did the provider actually return on that outbound call".

## What shipped in Task 1 — Audit logs

The append-only audit trail ships end-to-end.

- **Domain** (`internal/domain/audit/`) — `Entry` struct + nine `Action` constants (`integration.created`, `integration.deleted`, `integration.tested`, `message.sent`, `message.marked_read`, `conversation.marked_read`, `attachment.uploaded`, `user.logged_in`, `user.logged_out`). Sentinel errors: `ErrInvalidEntry`, `ErrInvalidCursor`.
- **Port** (`internal/ports/repository/audit.go`) — `AuditRepo` interface with `Record(ctx, Entry) (uint64, error)` and `List(ctx, orgID, AuditListFilter) ([]Entry, string, error)`.
- **Repository** (`internal/infrastructure/mysql/audit.go`) — implements `AuditRepo` against the `audit_logs` table seeded in migration `20260903000001`. Pagination: `(occurred_at, id)` tuple encoded as opaque base64.
- **Application service** (`internal/application/audit/service.go`) — `Service.Record` never returns an error; failures are logged so audit hiccups cannot break the caller's mutation. `Service.List` is a pass-through.
- **REST** (`internal/api/rest/v1/audit.go`) — `GET /api/v1/audit-logs` gated on `audit.read`. Filters: `resource_type`, `resource_id`, `action`, `actor_user_id`, `since`, `until`, `cursor`, `limit`.
- **RBAC** (`internal/domain/rbac/rbac.go`) — new `PermAuditRead` added to `All()`; admin roles inherit it via `Bootstrap.EnsureAdminRole` re-seeding.
- **OpenAPI** — schemas `AuditLog`, `AuditLogList`, `AuditLogFilter`; version bumped to `0.2.4-phase2`.
- **Frontend** — `web/src/routes/settings/audit.tsx` renders the filterable table; sidebar entry in `settings.tsx`.

### Follow-up commit (not this task)

The wire-up commit will:

- Register `AuditDeps` in `Deps` and mount the route from `router.go`.
- Construct `application/audit.Service` in `cmd/server/main.go`.
- Thread `Service.Record` calls into `application/integration`, `application/message` (send + read), `application/auth`, and the attachments handler.

Nothing in this task instruments existing services — the read surface is populated by seed data / manual inserts today and by the follow-up commit tomorrow.

## Task 2 — Provider-call telemetry (in flight)

Owned by the parallel agent. Deliverables: `internal/domain/provider_call/*`, `internal/infrastructure/mysql/provider_calls.go`, telemetry hook inside `internal/providers/whatsapp/*`, admin surfaces.

## What shipped in Task 3 — WhatsApp Template management

Templates land as a full vertical slice against Meta's `message_templates` API. Draft → submit → sync → delete all reachable through REST and a settings-panel UI.

- **Domain** (`internal/domain/template/`) — `Template` struct, `Category` + `Status` string enums, `Component` union struct. Sentinel errors `ErrNotFound`, `ErrInvalid`, `ErrIntegrationMissing`, `ErrNotSubmittable`.
- **Port** (`internal/ports/repository/templates.go`) — `TemplateRepo` interface (`Create`, `Get`, `List`, `Upsert`, `UpdateStatus`, `Delete`) + `TemplateListFilter` + `TemplatePage`.
- **Application service** (`internal/application/template/service.go`) — `Service.Create` (DRAFT with optional same-request submit), `SubmitForReview`, `Sync`, `Get`, `List`, `Delete`. Defines narrow `TemplateProvider` + `ProviderRegistry` ports so the application layer never imports any provider package.
- **Repository** (`internal/infrastructure/mysql/templates.go`) — `Templates` against the `templates` table; opaque base64 `(updated_at | id)` cursor pagination; `ON DUPLICATE KEY UPDATE` upsert on the natural key.
- **Provider adapter** (`internal/providers/whatsapp/templates.go`) — extended the stub with typed `ListTemplates` / `CreateTemplate` / `GetTemplateStatus` methods returning provider-native shapes (`TemplateSummary`, `TemplateCreateResult`, `TemplateStatus`). Existing client-level HTTP calls already trace through the Task 2 tracer.
- **REST** (`internal/api/rest/v1/templates.go`) — six routes behind auth + RBAC: `GET/POST /api/v1/templates`, `GET/DELETE /api/v1/templates/{id}`, `POST /api/v1/templates/{id}/submit`, `POST /api/v1/templates/sync`. RFC 7807 problem responses.
- **RBAC** (`internal/domain/rbac/perms_templates.go`) — `PermTemplatesRead` + `PermTemplatesManage`. Grant flows through migration `20260904000004_grant_templates_perms`.
- **Migrations** — `20260904000003_templates` (table) + `20260904000004_grant_templates_perms` (idempotent role backfill).
- **OpenAPI** — self-contained fragment at `internal/api/openapi/fragments/templates.yaml`.
- **Frontend** — `web/src/lib/templates.ts` TanStack Query hooks; `web/src/routes/settings.templates.tsx` list + wizard.
- **Docs** — `docs/domain/template.md`, `docs/flows/template-sync.md`.

### Follow-up commit (not this task)

- Register `TemplateDeps` on the REST `Deps` bundle and mount from `router.go`.
- Construct `application/template.Service` in `cmd/server/main.go` with the `ProviderRegistry` closure over `whatsapp.Provider`.
- Add `web/src/routes/settings.templates.tsx` to `web/src/router.tsx` and drop a sidebar link in `web/src/routes/settings.tsx`.
- Flip the Templates row in the WhatsApp capability matrix from stub to ✅.

## Task 4+ — RBAC UI, teams, delivery reports

Planned; see the master plan.
