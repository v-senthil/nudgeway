# OpenAPI CHANGELOG

Every change to `internal/api/openapi/openapi.yaml` gets an entry here.

## 0.1.0-phase0 — 2026-09-03

- Initial spec.
- Added `GET /healthz` — liveness probe.
- Added `GET /readyz` — readiness probe. Returns 503 when downstream deps unreachable (probes land Phase 0 Task 2).
- Defined `Problem` schema (RFC 7807) as the canonical error body.
- Defined `sessionCookie` + `apiKey` security schemes.
