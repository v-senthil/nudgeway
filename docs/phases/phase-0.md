# Phase 0 — Foundations

Status: **complete** (walking skeleton is real, end-to-end auth works, both servers boot).

## Goal

Repo is real. Local dev works against the developer's native MySQL + Redis (no Docker anywhere). A user can log in through the browser. Backend + frontend both green. Groundwork laid so Phase 1 can build the WhatsApp inbox.

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
- Go module `github.com/v-senthil/nudgeway` initialised.
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

## What shipped in Phase 0 Task 2 (auth infrastructure)

Commit `c3ed4e2`.

- **argon2id** password hashing (PHC-encoded, OWASP-2023 params) — `internal/infrastructure/auth/argon2.go`.
- **Session cookies** — `HttpOnly`, `SameSite=Lax`, `Secure` in prod. Opaque base64url session IDs; DB rows keyed by SHA-256(id) so the raw ID stays only in the cookie — `internal/infrastructure/auth/session.go`, `sessions.go` (mysql impl).
- **CSRF** double-submit cookie (JS-readable, echoed as `X-CSRF-Token`) with constant-time verify — `csrf.go`.
- **Middleware chain** — `RequestID → Recover → Logger → SessionAuth → RequireAuth → RequireCSRF` under `internal/infrastructure/http/middleware/`.
- **Permission resolver** — reads `role_permissions ⨝ user_roles` — `internal/infrastructure/mysql/rbac.go`.
- **Health probes** — MySQL + Redis; `/readyz` returns 503 with per-probe results when either is unreachable — `internal/infrastructure/health/`.
- **Redis client** — `internal/infrastructure/redis/client.go`.
- **In-memory session store** kept for tests (`memstore.go`); MySQL is the prod path.

## What shipped in Phase 0 Task 3 (login flow live)

Commit `c3ed4e2` (same batch — spawned 3 parallel agents).

- **REST v1 handlers**: `GET /api/v1/auth/csrf`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me` — RFC 7807 problem+json on errors.
- **Application service** `internal/application/auth.Service` — `Login` returns `ErrInvalidCredentials` on every failure mode (no user enumeration).
- **CLI subcommands**: `tenant create --slug --name`, `user create --org-slug --email --password --admin`, `migrate up|down|status`.
- **cmd/server/main.go rewritten** — wires config → MySQL → Redis → repos → application services → middleware chain → routes → probes → graceful shutdown in reverse-open order.
- **Frontend walking skeleton** — TanStack Router + Query; login page → `/inbox` three-pane layout → `/settings/integrations` cards; auth store with `useMe`/`useLogin`/`useLogout`; fetch wrapper auto-attaches CSRF header on non-GET; RFC 7807 → typed `ApiError`.
- **OpenAPI** additions: `LoginRequest`, `LoginResponse`, `Me` schemas + 4 auth ops.

Commit `a0d820c` — cleanup: stopped `tsc -b` from emitting `.js` next to `.tsx`; `tsconfig.json` set `noEmit: true`; scripts now `tsc --noEmit && vite build`.

Commit `3bc7132` — `/me` handler now returns `email` and `display_name` (fixed a runtime crash in the inbox Header when it called `.trim()` on undefined).

## End-to-end demo (verified 2026-09-03)

Both servers boot against the developer's native MySQL 8.4 + Redis 7:

```bash
# One-time setup
cp config/example.yaml config/local.yaml   # then edit mysql.dsn + auth.credential_kek_hex
go run ./cmd/cli migrate up                # applies 20260903000001 + 20260903000002
go run ./cmd/cli tenant create --slug acme --name "Acme Co"
go run ./cmd/cli user create --org-slug acme --email you@acme.com --password 'password123' --admin

# Run
go run ./cmd/server                        # :8080
cd web && npm run dev                      # :5173, proxies /api → :8080
```

Open http://localhost:5173/ → land on `/login` → sign in → land on `/inbox`. `curl -c cookies.txt /api/v1/auth/csrf` → `POST /login` → `GET /me` returns the user + org + permissions.

Live checks:
- `/healthz` → `{"status":"ok"}`
- `/readyz` → `{"status":"ready","probes":[{mysql ok},{redis ok}]}`
- `/api/v1/auth/me` → full principal

## Exit criteria — met

- ✅ `go run ./cmd/server` boots against real MySQL + Redis.
- ✅ Admin can log in from the browser.
- ✅ Health + readiness probes return correctly (503 on downstream outage).
- ✅ Backend + frontend both green (`go build ./...`, `go vet ./...`, `go test ./...` all pass; `npm run typecheck && npm run build`).
- ✅ Repo pushed to `origin` (`v-senthil/whatsapp-cloud-api`).

## Deferred (rolls into Phase 1)

- Real Prometheus metrics wiring (placeholder only for now).
- gstack team-mode wiring — owner-executed one-time step, documented above.
- Coverage bumped to ≥80% domain (Phase 1 backfill per user's "working first, tests later" preference).

## Docs delivered this phase

- [`docs/architecture.md`](../architecture.md)
- [`docs/onboarding.md`](../onboarding.md)
- [`docs/phases/phase-0.md`](phase-0.md) (this file)
- [`docs/adr/0001-language-and-deps.md`](../adr/0001-language-and-deps.md) through `0008-documentation-strategy.md`

## Migration notes

- `20260903000001_organizations_users_roles` — baseline schema. Every table InnoDB / utf8mb4. All FKs declared. `web_sessions` includes an expiry index for the cleanup job.
