# REST API

Every REST endpoint under `/api/v1/*` is generated from `internal/api/openapi/openapi.yaml`. Requests are scoped to the caller's organization; there is no cross-tenant access.

## Base URL

Local dev:

```
http://localhost:8080
```

The Vite frontend on `:5173` proxies `/api/*` and `/webhooks/*` to `:8080` — browser calls go through the frontend origin.

## Authentication

Two modes, mutually exclusive per request. Pick one:

### 1. Session cookie (browser)

- `POST /api/v1/auth/login` with `{email, password}` sets an `HttpOnly` + `Secure` + `SameSite=Lax` cookie named `nudgeway_session`.
- A companion `nudgeway_csrf` cookie (readable by JS) is set for the double-submit check.
- Every state-changing request (`POST` / `PUT` / `PATCH` / `DELETE`) must send `X-CSRF-Token: <value>` matching the cookie.
- Cookies are 30-day sliding.

### 2. Bearer token

- `Authorization: Bearer nk_<8-char-prefix>_<40-char-secret>` on every request.
- **No CSRF header required** — the bearer middleware short-circuits the double-submit check.
- Mint tokens via the UI (`/settings/api-tokens`) or the `createAPIToken` tool. See [Create a token](/#/api-tokens/create-token).

If both are sent, the server takes the cookie path and enforces CSRF. Choose one per request.

## Error shape (RFC 7807)

All errors are `application/problem+json`:

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "name: required",
  "instance": "/api/v1/api-tokens"
}
```

Field notes:

- `type` — stable URI, defaults to `about:blank`.
- `title` — human-readable status text.
- `status` — HTTP status echoed for convenience.
- `detail` — actionable message, safe to surface in a UI toast.
- `instance` — request path, useful when correlating with server logs by `request_id`.

## Observability

Every request gets a `request_id` (middleware `internal/infrastructure/http/middleware/requestid.go`) and is logged with structured slog fields including `org_id`, `request_id`, `correlation_id` where present. Latency is recorded as a Prometheus histogram + OpenTelemetry span.

## Health probes

- `GET /healthz` — liveness, always 200 while the process is up.
- `GET /readyz` — readiness, 200 only when MySQL / Redis / HBase / Kafka are all reachable.

## Related

- [OpenAPI spec](/#/developer/openapi) — the source of truth.
- [MCP server](/#/developer/mcp-server) — every operation as a tool.
- [API tokens overview](/#/api-tokens/overview) — bearer credentials + RBAC.
- [Meta API execution log](/#/audit-telemetry/provider-calls) — outbound sister log.
