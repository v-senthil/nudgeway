# ADR 0008 — Documentation strategy

Status: Accepted (2026-09-03)

## Context

Documentation rots. AI-assisted development can produce lots of code fast; unless docs are treated as a shipping artefact, we won't be able to onboard humans or agents to the codebase in six months.

## Decision

Every phase closes with:

- `docs/phases/phase-N.md` — what shipped, why, screenshots, curl examples, migration notes.
- `docs/domain/<entity>.md` — for every new domain entity (purpose, invariants, state machine, ERD snippet, code locations, Mermaid sequence).
- `docs/flows/<flow>.md` — for every new async flow (sequence diagram, event chain, retry/idempotency, failure modes, observability signals).
- `docs/providers/<provider>.md` — for every new adapter (capability matrix, mapped operations, webhook events, rate limits, credential schema, source-of-truth links).
- `docs/api/CHANGELOG.md` — every OpenAPI change.
- `docs/adr/NNNN-*.md` — one ADR per non-trivial choice (Nygard format).

CI runs a `docs-lint` step that fails if a new domain package or provider adapter has no matching doc file. Code-level: every exported symbol has a doc comment; every non-trivial function has a `// Overview:` opening paragraph.

## Consequences

- Docs are code-adjacent and code-reviewed.
- New engineers (human or AI) can bootstrap on the docs alone.
- If it isn't documented, it isn't shipped.
