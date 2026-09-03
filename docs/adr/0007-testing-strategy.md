# ADR 0007 — Testing strategy

Status: Accepted (2026-09-03)

## Context

Provider-agnostic platforms live and die by their tests. Adapters are the risk surface; core domain must be watertight; e2e must catch layout collapses.

## Decision

- **Unit tests** — table-driven, next to each file. Coverage ≥80% on `internal/domain/*` and `internal/application/*`; ≥60% overall.
- **Integration tests** (`//go:build integration`) — run against the developer's real MySQL / Redis / HBase in a dedicated `fullwa_test` schema/namespace, created + dropped per run. **No DB mocks.**
- **Provider adapter tests** — httptest fixtures captured from official docs (e.g. `~/Documents/whatsapp_doc_tracker/docs/`), never fabricated payloads.
- **Contract tests** — every REST response validated against the OpenAPI schema.
- **e2e** — Playwright golden-path per phase; runs headless in CI, headed locally.
- **Frontend** — Vitest + React Testing Library (≥70% on `web/src/features/*`); Playwright for e2e.
- **Coverage gate** in CI; fails if it drops on a changed package.

## Consequences

- Real infra required for integration tests — matches the deployment model (no Docker either).
- Provider tests document Meta/Zoho/etc.'s wire format inline, so regressions surface immediately.
- Contract tests keep the spec honest.
