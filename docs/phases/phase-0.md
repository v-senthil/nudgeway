# Phase 0 — Foundations

Status: **in progress**.

## Goal

Repo is real. gstack is installed. One canonical event can flow end-to-end. Green CI. Green `make verify`.

## What shipped in Phase 0 Task 1

- Repo initialised, MIT license, `README.md`, `.gitignore`.
- Full **CLAUDE.md harness** (17 sections — the AI operating manual).
- **No Docker, no Kubernetes.** Local infra is the user's already-installed MySQL / Redis / HBase, wired via `config/local.yaml`.
- `config/example.yaml` — committed template.
- `scripts/check-infra.sh` — verifies MySQL / Redis / HBase are reachable before `make dev`.
- `scripts/dsn-from-config.sh` — extracts the MySQL DSN for `golang-migrate`.
- `Makefile` — `check-infra`, `deps`, `tools`, `fmt`, `vet`, `lint`, `lint-openapi`, `gen`, `gen-sqlc`, `gen-api`, `test`, `test-int`, `test-frontend`, `e2e`, `coverage-check`, `verify`, `migrate`, `build`, `frontend`, `dev`, `clean`.
- Directory layout locked to `docs/architecture.md`:
  - `cmd/{server,cli}`
  - `internal/domain/{organization,user,rbac,contact,identity,session,conversation,message,ticket,template,campaign,automation,ai,call,integration,events}`
  - `internal/application/{contact,conversation,message,ticket,campaign,automation,ai}`
  - `internal/ports/{repository,queue,eventbus,attachments,channel,ticketing,bot,aiport,calling}`
  - `internal/providers/{whatsapp,zoho_desk,openai,anthropic,zoho_zia}`
  - `internal/infrastructure/{mysql,redis,hbase,websocket,http,auth,logging,metrics,tracing}`
  - `internal/{events,workers,scheduler,webhook,api/{rest/v1,ws,openapi}}`
  - `web/` (Vite + React + TS + Tailwind)
- Go module `github.com/fullwa/fullwa` initialised.
- **Skeleton with real content, not just stubs**:
  - `cmd/server/main.go` — boots stdlib HTTP server, `/healthz`, `/readyz`, `/metrics`, structured slog, graceful shutdown.
  - `internal/infrastructure/config/` — YAML loader + env overrides + validation, unit-tested against `config/example.yaml`.
  - `internal/infrastructure/http/` — server wrapper, unit-tested.
  - `internal/domain/events/` — 40+ canonical event types + `Envelope`, unit-tested.
  - `internal/events/inproc.go` — synchronous in-process event bus, unit-tested (fan-out + first-error).
  - `internal/ports/*` — `channel.Provider`, `ticketing.Provider`, `bot.Provider`, `aiport.Provider`, `calling.Provider`, `eventbus.{Publisher,Subscriber}`, `queue.{Enqueuer,Consumer}`, `attachments.Store`.
  - `internal/providers/registry.go` — self-registering provider registry, unit-tested (register + lookup + duplicate panic).
- **First migration** — `20260903000001_organizations_users_roles.{up,down}.sql`: organizations, users, teams, roles, role_permissions, user_roles, web_sessions, audit_logs.
- **OpenAPI 3.1 spec skeleton** — `internal/api/openapi/openapi.yaml` — `/healthz`, `/readyz`, Problem schema (RFC 7807), session-cookie + API-key security schemes.
- **Vite React scaffold** — `web/` with TypeScript strict + noUncheckedIndexedAccess + exactOptionalPropertyTypes, TanStack Router/Query deps, Tailwind, a first App component that hits `/healthz`, a Vitest test.
- **CI** — `.github/workflows/ci.yml` — backend (vet, golangci-lint, go-arch-lint, tests + coverage against runner-hosted MySQL + Redis), OpenAPI (Spectral), frontend (typecheck + build). No containers in the workflow itself.
- **Lint config**:
  - `.golangci.yml` — strict linter set (errcheck, gosec, govet, staticcheck, revive, gocritic, gocyclo≤15, funlen≤80, dupl, misspell, sqlclosecheck, rowserrcheck, contextcheck, nilerr, wastedassign).
  - `.go-arch-lint.yml` — dependency rule enforcement.
  - `.spectral.yaml` — OpenAPI ruleset.
  - `sqlc.yaml` — sqlc config for typed queries.

## Manual step (owner-executed)

gstack install requires cloning `garrytan/gstack` and running its `setup` script — third-party code executed on the developer's machine. Left as a one-time manual action for the repo owner. Run once:

```bash
git clone --single-branch --depth 1 https://github.com/garrytan/gstack.git ~/.claude/skills/gstack
cd ~/.claude/skills/gstack && ./setup && ./setup --team
~/.claude/skills/gstack/bin/gstack-team-init required
# then, from this repo:
git add .claude/ CLAUDE.md
git commit -m "chore: enable gstack team mode"
```

After that, every Claude Code session on this repo runs through the gstack skill pipeline referenced in [`CLAUDE.md` §12](../../CLAUDE.md#12-gstack-workflow).

## What lands in Phase 0 Task 2 (next)

- Complete gstack team-mode wiring (post owner install).
- Session-cookie auth + argon2id + CSRF middleware.
- RBAC middleware + permission-key checks.
- Login screen wired to the API.
- `sqlc` queries for organizations / users / roles.
- `/readyz` probes MySQL + Redis + HBase.
- Coverage bumped to ≥60% overall / ≥80% domain.
- Full CI green.

## Exit criteria (Phase 0 overall)

- `make dev` starts a browsable app.
- An admin can log in.
- `/healthz` + `/readyz` return 200.
- Coverage gate passes.
- CI green on `main`.

## Docs delivered this phase

- [`docs/architecture.md`](../architecture.md)
- [`docs/onboarding.md`](../onboarding.md)
- [`docs/phases/phase-0.md`](phase-0.md) (this file)
- [`docs/adr/0001-language-and-deps.md`](../adr/0001-language-and-deps.md) through `0008-documentation-strategy.md`

## Migration notes

- `20260903000001_organizations_users_roles` — baseline schema. Every table InnoDB / utf8mb4. All FKs declared. `web_sessions` includes an expiry index for the cleanup job.
