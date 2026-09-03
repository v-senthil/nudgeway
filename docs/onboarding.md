# fullWA — engineer onboarding

Welcome. This is the path from `git clone` to your first message-sent.

## 1. Prereqs (local, native — no Docker)

- **Go** — install via `brew install go` or the official installer. Any recent stable version.
- **Node.js 20+** — `brew install node` or `nvm install 20`.
- **MySQL 8+** — `brew install mysql && brew services start mysql`.
- **Redis 7+** — `brew install redis && brew services start redis`.
- **HBase 2+** — `brew install hbase` or download from https://hbase.apache.org/. Start with `start-hbase.sh`.

## 2. Configure

```bash
cp config/example.yaml config/local.yaml
```

Edit `config/local.yaml` and set:
- `mysql.dsn` — match your local MySQL user/password/port.
- `redis.addr` — usually `127.0.0.1:6379`.
- `hbase.zookeeper_quorum` — usually `["127.0.0.1:2181"]`.
- `auth.credential_kek_hex` — generate one with `openssl rand -hex 32`.

## 3. Verify

```bash
make check-infra
```

Green output means MySQL, Redis, and HBase are all reachable. If anything is red, start the missing service before continuing.

## 4. First run

```bash
make migrate up      # applies schema migrations to the local MySQL
make dev             # runs the Go server + Vite dev server
```

Open http://localhost:8080. You should see the Phase 0 walking-skeleton page and a green "backend ok" indicator.

## 5. Where things live

- **Backend Go** — `cmd/`, `internal/`. See [`docs/architecture.md`](architecture.md) and the dependency rule in [`CLAUDE.md`](../CLAUDE.md#4-dependency-rule).
- **Frontend** — `web/`.
- **OpenAPI spec** — `internal/api/openapi/openapi.yaml`. Every REST endpoint is spec-first.
- **Migrations** — `migrations/`. Use `make migrate` to apply.
- **Docs** — `docs/`. ADRs under `docs/adr/`, phase notes under `docs/phases/`, per-entity docs under `docs/domain/`, per-flow under `docs/flows/`, per-provider under `docs/providers/`.

## 6. Before you commit

```bash
make verify
```

That runs the same gates as CI: fmt, vet, golangci-lint, go-arch-lint, spectral, unit tests + coverage, frontend build.

## 7. Conventions

- Read [`CLAUDE.md`](../CLAUDE.md) end-to-end before your first change. It is the operating manual.
- Conventional Commits: `feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`.
- One logical change per commit; PR body links its phase task + any ADR it touches.
- Spec-first API changes: edit `openapi.yaml` → `make gen-api` → implement handler → contract test → doc.

## 8. Common pitfalls

- **Don't import provider SDKs outside `internal/providers/*`.** `go-arch-lint` will fail your build. Model the operation on a port instead.
- **Don't hold a DB transaction while calling an external API.** Persist, commit, enqueue.
- **Don't trust `organization_id` from the client.** Read it from the authenticated principal.

## 9. Getting help

- Architecture questions → [`docs/architecture.md`](architecture.md) + [`docs/adr/`](adr/).
- Domain model questions → [`docs/domain/`](domain/).
- Provider quirks → [`docs/providers/`](providers/) + the vendor docs mirror at `~/Documents/whatsapp_doc_tracker/docs/` for Meta.
- When in doubt → open an `[RFC]` PR.
