---
name: nudgeway-api-tokens
description: Nudgeway API tokens — mint, list, and revoke opaque bearer credentials used by the MCP server, CLI clients, and third-party integrations. Tokens are `argon2id`-hashed at rest; the plaintext is returned exactly once.
trigger: User asks about API keys, API tokens, bearer credentials, personal access tokens, wiring the MCP server auth, or replacing session-cookie auth for scripted API access.
---

# Nudgeway API tokens skill

## Overview

An **API token** is an opaque bearer credential scoped to the minting user's org and RBAC permissions. It is the recommended auth path for the MCP server (`./bin/nudgeway-mcp`), the CLI, and any external script that needs to hit `/api/v1/*` without a browser session.

Plaintext shape: `nk_<8-char-prefix>_<40-char-secret>` (base32).
- The **prefix** is stored in cleartext and is used as the lookup key.
- The **secret** is `argon2id`-hashed at rest — never persisted or logged in plaintext.
- The full plaintext token is returned **exactly once**, in the create response. If lost, revoke and mint a new one.

Bearer path skips CSRF: when the request carries `Authorization: Bearer <token>` the backend middleware does not require the `X-CSRF-Token` double-submit.

## MCP tools

| operationId | Purpose |
|---|---|
| `createAPIToken` | Mint a new token. Body: `{name, expires_at?}`. Response includes the plaintext `token` once. |
| `listAPITokens` | List tokens visible to the caller (metadata only — no plaintext). |
| `revokeAPIToken` | Revoke a token by ID. Idempotent. |
| `getAPITokenUsage` | Paginated execution log for a single token — one row per bearer request. Query: `from`, `to`, `limit`, `cursor`. |
| `getAPITokenMetrics` | KPIs (call count, error rate, p50/p95 latency), per-day time series, and top request paths for a single token. Query: `from`, `to`. |

## REST equivalents

```
POST   /api/v1/api-tokens
GET    /api/v1/api-tokens
DELETE /api/v1/api-tokens/{id}
GET    /api/v1/api-tokens/{id}/usage
GET    /api/v1/api-tokens/{id}/metrics
```

The mint / list / revoke trio is guarded by the `api_tokens.manage` RBAC scope. The two read-only usage endpoints are guarded by `api_tokens.read` (users can always read their own token's usage; org admins can read any token in the org).

## Usage tracking

Every request that authenticates via `Authorization: Bearer <token>` is captured in the `api_token_usage` execution log. The middleware wraps `ResponseWriter`, records:

- `occurred_at` — UTC timestamp of the request.
- `remote_ip` — client IP (honours `X-Forwarded-For` from the trusted-proxy list).
- `method` + `path` — HTTP verb and route template (path template, not the fully-substituted URL — keeps cardinality bounded).
- `request_body` — raw bytes, capped at 8 KiB, JSON-redacted (see the Gotcha below).
- `response_body` — raw bytes, capped at 8 KiB, same redaction rules; `null` for media/binary downloads.
- `status_code` — final HTTP status returned.
- `latency_ms` — wall-clock request duration.

Writes happen on a detached goroutine after the response is flushed, so the log never blocks or fails the request. Rows roll up nightly into `api_token_usage_daily` (call count, error count, latency percentiles, top-path histogram) to keep the metrics endpoint fast on long windows.

**Fetch via REST**

```
GET /api/v1/api-tokens/{id}/usage?from=2026-09-04T00:00:00Z&to=2026-09-05T00:00:00Z&limit=50
GET /api/v1/api-tokens/{id}/metrics?from=2026-08-06T00:00:00Z&to=2026-09-05T00:00:00Z
```

**Fetch via MCP** — see the Common patterns section below.

**Retention** — currently unlimited. A Phase 4 pruning worker will drop `api_token_usage` rows older than 30 days (operator-configurable) while keeping the daily rollup indefinitely, mirroring the pattern in [`docs/domain/provider_call.md`](../../docs/domain/provider_call.md#retention). Until that lands, operators should watch table size and periodically `DELETE` older rows manually.

## Common patterns

### Mint a token for a new MCP client

```json
{
  "tool": "createAPIToken",
  "arguments": {
    "body": {
      "name": "claude-desktop-laptop",
      "expires_at": "2027-01-01T00:00:00Z"
    }
  }
}
```

Response (once, then never again):

```json
{
  "id": "01JC5...",
  "name": "claude-desktop-laptop",
  "prefix": "nk_abcd1234",
  "token": "nk_abcd1234_<40-char-secret>",
  "created_at": "…",
  "expires_at": "2027-01-01T00:00:00Z"
}
```

Paste `token` into the MCP client's `NUDGEWAY_API_TOKEN` env var.

### Rotate

1. `createAPIToken` with a new name (e.g. append `-2`).
2. Update the client env / config with the new plaintext.
3. `revokeAPIToken` on the old token's `id`.

### Audit

`listAPITokens` returns metadata (`id`, `name`, `prefix`, `created_at`, `last_used_at`, `expires_at`, `revoked_at`) — pair with `getAuditLogs` to trace usage over time.

### Fetch the last 24 hours of usage for a specific token

Given the token's `id` (from `listAPITokens`), call `getAPITokenUsage` with an ISO-8601 window that ends now:

```json
{
  "tool": "getAPITokenUsage",
  "arguments": {
    "id": "01JC5XYZTOKENID",
    "from": "2026-09-04T09:00:00Z",
    "to":   "2026-09-05T09:00:00Z",
    "limit": 100
  }
}
```

Response is a paginated list of execution-log rows (`occurred_at`, `remote_ip`, `method`, `path`, `status_code`, `latency_ms`, `request_body`, `response_body`) plus a `next_cursor` when more pages exist. For summary KPIs across the same window, call `getAPITokenMetrics` with the same `from`/`to` — it returns call counts, error rate, latency percentiles, a per-day series, and the top request paths.

## Gotchas

- **Plaintext is shown once.** No recovery flow. Lost token → revoke + remint.
- **Bearer skips CSRF.** State-changing calls succeed without an `X-CSRF-Token` header when authenticated via bearer. The session-cookie path still enforces CSRF.
- **RBAC inherits from the minting user.** A token can never do more than its owner. Revoking the owner's user account revokes the tokens.
- **Prefix is public-ish.** It appears in `listAPITokens` and audit logs — treat it as an identifier, not a secret. The secret half is what matters.
- **No wildcard token.** There is no org-wide or admin-bypass token; every token is bound to a user.
- **Usage bodies are capped + redacted.** Request and response bodies stored in `api_token_usage` are truncated at 8 KiB per direction. When the payload parses as JSON, the keys `password`, `access_token`, `app_secret`, `verify_token`, `secrets`, `plaintext`, `token`, and `secret` are replaced with the string `"[redacted]"` before persistence. Media / binary responses store `response_body: null` and record only the byte size. Do not rely on the usage log to reconstruct full payloads for large or binary responses — use the provider-call log or application traces for that.

## Related skills

- [`../nudgeway-mcp/SKILL.md`](../nudgeway-mcp/SKILL.md) — how the MCP forwarder consumes `NUDGEWAY_API_TOKEN`.
- [`../nudgeway-integrations/SKILL.md`](../nudgeway-integrations/SKILL.md) — provider credentials (Meta access tokens etc.) are separate from Nudgeway API tokens; do not confuse the two.
