# fullWA CHANGELOG

Human-readable project history. Commit hashes are on `main` at `v-senthil/whatsapp-cloud-api`. For the OpenAPI-specific changelog see [`docs/api/CHANGELOG.md`](docs/api/CHANGELOG.md).

Format: reverse-chronological. Latest at the top.

---

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
