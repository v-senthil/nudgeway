# OpenAPI CHANGELOG

Every change to `internal/api/openapi/openapi.yaml` gets an entry here.

## 0.1.1 — 2026-09-04

- **`Me` schema** now includes required `email` and `display_name`. Fixes a frontend inbox crash where the initials helper called `.trim()` on undefined. (`3bc7132`)

## 0.1.0-phase0-auth — 2026-09-03

- Added `GET /api/v1/auth/csrf` — issues the double-submit CSRF cookie for the first login.
- Added `POST /api/v1/auth/login` — email + password login; sets session + CSRF cookies.
- Added `POST /api/v1/auth/logout` — invalidates the session; clears cookies. Requires CSRF.
- Added `GET /api/v1/auth/me` — returns the current principal, org, and permissions.
- Added `LoginRequest`, `LoginResponse`, `Me` schemas.

## 0.1.0-phase0 — 2026-09-03

- Initial spec.
- Added `GET /healthz` — liveness probe.
- Added `GET /readyz` — readiness probe. Returns 503 when downstream deps unreachable (probes land Phase 0 Task 2).
- Defined `Problem` schema (RFC 7807) as the canonical error body.
- Defined `sessionCookie` + `apiKey` security schemes.
