# CLAUDE.md — Nudgeway operating manual for AI-assisted work

This file is the AI operating manual for every future Claude Code (or other agent) session on this repo. It is not a stub. Read it fully before making changes.

<!-- gstack:verify: make verify -->

---

## 1. Project overview

**Nudgeway** is an open-source, multi-tenant, provider-agnostic customer communication and engagement platform, initially centred on the WhatsApp Business Cloud API. Backend is a Go modular monolith compiled to a single binary; frontend is a Vite + React + TypeScript SPA embedded via `//go:embed`. MySQL is the transactional source of truth, Redis handles queues/cache/coordination, HBase stores high-volume message + event data. See [`docs/architecture.md`](docs/architecture.md) for the full picture and [`docs/phases/`](docs/phases/) for what's shipped.

---

## 2. Prime directives (non-negotiable)

1. **Canonical domain → Persist → Event → Async processing → Provider adapter → Result → Event → Real-time UI.** This is the shape of every feature.
2. **Contact ≠ Session ≠ Conversation ≠ Message ≠ Ticket.** All are separate first-class entities.
3. **Never hold a DB transaction while calling an external API.** Persist, commit, then enqueue.
4. **No provider leakage into core domain packages.** OpenAI / Anthropic / etc. live *only* inside `internal/providers/*`.
5. **Multi-tenancy is enforced at every query layer.** Never trust `organization_id` from the client.
6. **Idempotency on every webhook + every external send.**
7. **Persist authoritative state before async processing.**
8. **Single binary, modular monolith.** No microservices without a proven scaling reason.

Violation of any of these blocks merge.

---

## 3. Repo map

```
cmd/server/               single-binary entrypoint (HTTP + WS + workers + scheduler)
cmd/cli/                  admin CLI (migrate, seed, tenant create, user invite, key rotate)
internal/domain/          pure Go domain model — zero infra, zero provider imports
internal/application/     use-cases; orchestrates domain + ports
internal/ports/           interfaces the application depends on
internal/providers/       the ONLY place third-party SDKs live
internal/infrastructure/  MySQL, Redis, HBase, HTTP, auth, observability
internal/events/          event bus (in-proc + Redis Streams)
internal/workers/         background consumers
internal/scheduler/       cron-style + delayed job dispatcher
internal/webhook/         provider-agnostic webhook ingress
internal/api/rest/        REST handlers (generated from openapi.yaml)
internal/api/ws/          WebSocket endpoints
internal/api/openapi/     OpenAPI 3.1 spec (source of truth)
web/                      Vite + React frontend, embedded via //go:embed
migrations/               golang-migrate SQL files
config/                   example.yaml (committed); local.yaml (git-ignored)
docs/                     architecture, ADRs, phases, domain, flows, providers, api, observability, runbook
scripts/                  dev helpers, load-test, seed data, infra checks
```

---

## 4. Dependency rule

Enforced by `.go-arch-lint.yml` + CI grep guards. Violations fail CI.

```
domain          → stdlib + domain siblings only
application     → domain + ports
infrastructure  → ports (implements them) + stdlib + drivers
providers       → ports (implements them) + provider SDKs
cmd             → wires everything together
```

If you catch yourself importing `internal/providers/whatsapp` from `internal/application/*` or `internal/domain/*` — stop. Model the operation on a port instead.

---

## 5. How to run

Prereqs (local, native, **no Docker**): MySQL 8+, Redis 7+, HBase 2+, **Kafka 3+** running on your machine.

Kafka is used for durable event log + cross-node fan-out and job queues (per-conversation ordering via partition key). Redis stays for cache, distributed locks, rate limiters, idempotency, and short-lived state.

```bash
cp config/example.yaml config/local.yaml   # edit to point at your local services
make check-infra                           # verify MySQL / Redis / HBase reachable
make migrate up                            # apply migrations
make dev                                   # Go server + Vite frontend
```

Open http://localhost:8080.

---

## 6. How to test

```bash
make test         # unit tests
make test-int     # integration tests against local MySQL/Redis/HBase (creates + drops nudgeway_test schema/namespace)
make e2e          # Playwright golden path for the current phase
make verify       # everything CI runs: fmt, vet, lint, arch-lint, sqlc-diff, spectral, openapi-diff, unit, int, frontend
```

**Coverage targets** (enforced in CI):
- `internal/domain/*` ≥ 80%
- `internal/application/*` ≥ 80%
- `web/src/features/*` ≥ 70%
- Overall Go ≥ 60%

Where to add tests:
- `_test.go` next to each Go file for units.
- `*_integration_test.go` with `//go:build integration` for integration.
- `web/src/**/*.test.ts(x)` for Vitest.
- `e2e/*.spec.ts` for Playwright.

**Do not mock the database.** Integration tests hit real MySQL / Redis / HBase. Third-party HTTP (Meta, OpenAI) is mocked via `httptest` + fixtures.

---

## 7. How to add a provider

Adding a new provider must NOT require changing `internal/domain/*` or `internal/application/*`.

1. Create `internal/providers/<name>/`.
2. Implement the relevant port interface(s) from `internal/ports/{channel,ticketing,bot,ai,calling}/`.
3. Implement webhook parsing/signature verification in `internal/providers/<name>/webhook.go`.
4. Declare capabilities in `capabilities.go`.
5. Register the provider in `internal/providers/registry.go` (via `init()`).
6. Add integration UI wiring under `web/src/routes/settings/integrations/`.
7. Add:
   - Integration tests using real HTTP fixtures under `internal/providers/<name>/testdata/`.
   - `docs/providers/<name>.md` covering capabilities, mapped operations, webhook events, rate limits, credentials.

---

## 8. How to add an API endpoint

**Spec-first.** Never hand-write handler signatures.

1. Edit `internal/api/openapi/openapi.yaml`. Add request, response, error schemas. Add operation with `operationId`, `security`, `tags`.
2. Run `make gen-api` — regenerates Go server interfaces (`oapi-codegen`) and TS client (`openapi-typescript` + `openapi-fetch`).
3. Implement the handler by satisfying the generated interface in `internal/api/rest/v1/<resource>.go`. Handler calls into `internal/application/*` — never directly into `internal/infrastructure/*` or `internal/providers/*`.
4. Add contract test asserting real response validates against the OpenAPI schema.
5. Update `docs/api/CHANGELOG.md`.
6. Wire the frontend hook using the generated client.
7. The MCP server auto-generates tools from openapi.yaml at boot, so no code change is needed for MCP — but you MUST verify the new operation appears in the MCP tool list (`./bin/nudgeway-mcp --list-tools` or via an MCP client).

Errors: RFC 7807 `application/problem+json` always.

---

## 9. How to add a migration

1. Create `migrations/NNNNNNNN_short_name.up.sql` and `.down.sql`. Use the next timestamp-based number.
2. Migrations must be reversible and safe to re-run (`IF NOT EXISTS`, `IF EXISTS`).
3. Every table carries `org_id` as the first column of every non-primary index.
4. Every schema change is reviewed by `sqlfluff` (`make lint-sql`).
5. Add a note to `docs/phases/phase-N.md` describing the change.
6. `make migrate up` applies locally; `sqlc generate` regenerates typed queries.

---

## 10. How to add a domain event

1. Define the event struct in `internal/domain/events/`. Include `OrgID`, `OccurredAt`, `CorrelationID`, `CausationID`.
2. Add its protobuf schema in `internal/events/proto/`.
3. Publish it from the application layer via the `Publisher` port.
4. Wire subscribers in `cmd/server/main.go`.
5. Document publisher + consumers in `docs/flows/<flow>.md`.
6. Add a unit test for serialisation and a subscriber test.

Ordering guarantee: per-`conversation_id` ordering is enforced by consumer-group key. If you need different ordering, discuss in an ADR before shipping.

---

## 11. Anti-patterns (CI blocks these)

- `if provider == "whatsapp"` (or any provider string) inside `internal/domain/*` or `internal/application/*`.
- Importing OpenAI / Anthropic SDKs anywhere outside `internal/providers/*`.
- Importing `database/sql`, `github.com/redis/go-redis`, or `github.com/tsuna/gohbase` inside `internal/domain/*` or `internal/application/*`.
- `context.Background()` inside a request handler or worker (must propagate the incoming ctx).
- Unbounded `go func(){}` anywhere except `internal/workers/pool.go`.
- Storing JWTs in `localStorage`/`sessionStorage` — use HTTP-only cookies.
- Cross-tenant reads (any query missing `org_id`).
- Hand-written SQL outside `sqlc` queries.
- Holding a DB tx while making an outbound HTTP call.
- OpenAPI + REST handler + MCP tool must ship together. Handler without spec entry, or spec entry without handler, are both blocked.

---

## 12. gstack workflow

This repo is set up with **gstack team mode**. Every AI-assisted session should follow:

1. `/office-hours` — reframe / pressure-test the ask before coding.
2. `/autoplan` — produce a plan.
3. Implement.
4. `/review` — self-review before opening a PR.
5. `/qa` — browser-driven verification of the golden path.
6. `/ship` — open the PR.

The `<!-- gstack:verify: make verify -->` line at the top of this file makes `/ship` block until `make verify` is green.

Reference: `~/.claude/skills/gstack`.

---

## 13. Definition of done

A feature is not done until *all* of these ship together:

**Backend**
- OpenAPI spec updated + regenerated.
- Handler + application service + domain code, each with tests.
- Migration if applicable.
- Idempotency + retry for any external call.
- Structured slog logging with `org_id`, `request_id`, `correlation_id`.
- Prometheus metric + OpenTelemetry span.
- Canonical event published if the feature changes observable state.

**Frontend**
- Client hook generated from OpenAPI.
- Loading, empty, error, permission-denied, offline states.
- WebSocket wire-up for real-time updates where applicable.
- Vitest unit tests + at least one component test.

**Docs (MANDATORY — no code change is complete without them)**
- `CHANGELOG.md` at repo root — one entry per commit that changes behaviour. Human-readable "why + what shipped".
- `docs/phases/phase-N.md` — every commit that advances a phase updates the "What shipped in Task X" section here.
- `docs/domain/<entity>.md` if a new domain entity or invariant.
- `docs/flows/<flow>.md` if a new async flow.
- `docs/providers/<provider>.md` if a new provider.
- `docs/api/CHANGELOG.md` entry for every OpenAPI change (bump the version line).
- ADR if a non-trivial choice.
- **Claude memory update** in `~/.claude/projects/-Users-senthil-11424-Documents-Nudgeway/memory/`:
  - `project_nudgeway_state.md` — current phase, seeded state, live services, next planned batch.
  - `reference_repo_and_docs.md` — if a new code entry point or docs location was added.
  - New `feedback_*.md` if a user preference or correction was discovered.

The rule is simple: **if a git diff changes runtime behaviour, at least three files land in the same commit — the code change, the CHANGELOG entry, and the phase doc update.** No exceptions except pure typo fixes.

---

## 14. Coding conventions

**Go**
- `gofmt` + `goimports`.
- `golangci-lint` with the config in `.golangci.yml` (errcheck, gosec, govet, staticcheck, revive, gocritic, gocyclo≤15, funlen≤80, dupl, contextcheck, nilerr, wastedassign, sqlclosecheck, rowserrcheck).
- Errors are wrapped: `fmt.Errorf("verb noun: %w", err)`. No bare `return err` when adding context.
- Every exported symbol has a doc comment starting with the symbol name.
- Package names: lowercase, single word.
- No global mutable state. Constructor injection only.
- `context.Context` is the first arg of every function that may block.

**TypeScript**
- `strict: true`, `noUncheckedIndexedAccess: true`, `exactOptionalPropertyTypes: true`.
- ESLint (`@typescript-eslint/recommended-type-checked`, `react-hooks`, `tanstack/query`, `tanstack/router`).
- No `any`. No `as` casts except at API/DOM boundaries.
- Data-fetching via generated `openapi-fetch` client + TanStack Query.

**SQL**
- All queries through `sqlc`-generated code. Hand-written `db.Query` is banned.
- Every migration reversible + linted.

**Commits**
- Conventional Commits: `feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`.
- One logical change per commit.
- PR body links its phase task + any relevant ADR / spec section.

---

## 15. Security rules

- Session cookies: `HttpOnly`, `Secure`, `SameSite=Lax`. CSRF double-submit cookie for state-changing requests.
- API tokens: opaque bearer credentials shaped `nk_<8-char-prefix>_<40-char-secret>`. Secret half is `argon2id`-hashed at rest; plaintext returned exactly once at creation. Sent as `Authorization: Bearer <token>`; the bearer middleware skips the CSRF double-submit. Tokens inherit the minting user's org + RBAC scopes and are revocable at `/settings/api-tokens`.
- Passwords: `argon2id` (params documented in ADR).
- Provider credentials: envelope-encrypted (KEK from env, DEK per-integration). Never in logs.
- Webhook signatures verified before any parsing.
- Outbound HTTP: SSRF blocklist (RFC 1918, link-local, metadata IPs); no user-supplied URLs without allowlist.
- Never log request bodies containing PII, tokens, or template variables.
- No `eval`-shaped constructs in the automation DSL.
- Every mutation writes an `AuditLog` row (see spec §50).
- Bearer-authenticated requests are recorded to the `api_token_usage` execution log by a middleware that wraps `ResponseWriter`, caps request + response bodies at 8 KiB per direction, redacts known secret JSON keys (`password`, `access_token`, `app_secret`, `verify_token`, `secrets`, `plaintext`, `token`, `secret`) to `"[redacted]"`, and writes on a detached goroutine so bookkeeping never blocks the response. See [`docs/api-token-usage.md`](docs/api-token-usage.md).

---

## 16. Observability rules

- Every request gets a `request_id` (middleware `internal/infrastructure/http/middleware/requestid.go`).
- Every job carries a `correlation_id` — set from the originating request or webhook.
- Every event carries a `causation_id` — the event or request that caused it.
- Every provider call records latency + outcome as a Prometheus histogram + OTel span.
- Every bearer-authenticated request produces an `api_token_usage` row keyed by `token_id` (timestamp, remote_ip, method, path, redacted request/response bodies, status code, latency_ms) with a daily rollup in `api_token_usage_daily`. See [`docs/api-token-usage.md`](docs/api-token-usage.md).
- Slog fields always include `org_id`, `request_id`, `correlation_id` where present.
- Traces stitch REST → worker → provider → webhook return.
- Health probes at `/healthz` (liveness) and `/readyz` (readiness — checks DB, Redis, HBase).

---

## 17. When in doubt

- Read `docs/architecture.md` and any relevant `docs/adr/*.md`.
- If your change requires a non-trivial choice not covered by an existing ADR, add a new ADR (`docs/adr/NNNN-title.md`) as part of the PR.
- Don't guess — ask, or open a discussion PR marked `[RFC]`.
- Reference source of truth for external APIs: `~/Documents/whatsapp_doc_tracker/docs/` (Meta), official docs for others. Never invent provider surfaces.

---

## Nudgeway skills

Repo-local skills at [`skills/`](skills/) — one per domain. Any Claude Code agent (or MCP-aware LLM) working against Nudgeway should read the relevant SKILL.md before touching the corresponding surface:

- [`skills/nudgeway-inbox`](skills/nudgeway-inbox/SKILL.md) — conversations, messages, read state
- [`skills/nudgeway-integrations`](skills/nudgeway-integrations/SKILL.md) — provider CRUD + webhook + test
- [`skills/nudgeway-templates`](skills/nudgeway-templates/SKILL.md) — template CRUD + sync
- [`skills/nudgeway-calls`](skills/nudgeway-calls/SKILL.md) — call flow + permissions
- [`skills/nudgeway-analytics`](skills/nudgeway-analytics/SKILL.md) — KPIs + sparklines + provider-call telemetry
- [`skills/nudgeway-mcp`](skills/nudgeway-mcp/SKILL.md) — how to run the MCP server + wire it into a client

See [`skills/README.md`](skills/README.md) for the golden rules that apply to every skill (multi-tenancy, CSRF, idempotency, RBAC, audit).

## gstack skills index

Available in every session (installed at `~/.claude/skills/gstack`):

`/office-hours`, `/autoplan`, `/plan-ceo-review`, `/plan-eng-review`, `/plan-design-review`, `/design-consultation`, `/design-shotgun`, `/design-html`, `/review`, `/ship`, `/land-and-deploy`, `/canary`, `/benchmark`, `/browse`, `/connect-chrome`, `/qa`, `/qa-only`, `/design-review`, `/setup-browser-cookies`, `/setup-deploy`, `/setup-gbrain`, `/retro`, `/investigate`, `/document-release`, `/document-generate`, `/codex`, `/cso`, `/autoplan`, `/plan-devex-review`, `/devex-review`, `/careful`, `/freeze`, `/guard`, `/unfreeze`, `/gstack-upgrade`, `/learn`.
