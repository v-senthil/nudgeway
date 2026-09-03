# ADR 0005 — Auth: session cookies + API keys

Status: Accepted (2026-09-03)

## Context

Two consumer classes: the web app (browser) and programmatic clients. Browser JS should never hold a JWT (XSS risk); server-side sessions are safer. Programmatic clients want simple headers.

## Decision

- **Web app** — HTTP-only, `SameSite=Lax`, `Secure` session cookies. CSRF via double-submit cookie for state-changing requests. Sliding TTL.
- **Programmatic** — `X-API-Key` header, scoped per org, with per-key permissions.
- Passwords hashed with **argon2id** (memory 64 MiB, iterations 3, parallelism 2 — revisit annually).
- OAuth (Google, Microsoft) planned as optional providers on the sign-in screen (Phase 0 Task 2+).

## Consequences

- No JWT in JS — the anti-pattern is closed off.
- CSRF and cookie flags are must-haves; enforced by middleware.
- Session table has an expiry index for the cleanup worker.
