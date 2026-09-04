# API token usage tracking

Every request that authenticates via `Authorization: Bearer <token>`
against a Nudgeway API token is recorded to a per-token execution log
plus a per-day rollup. This document describes what's captured, how the
middleware captures it, the data model, retention posture, and the
operator surfaces (UI + REST + MCP).

## What

Two tables, both scoped by `org_id` and joined via `token_id`:

- **`api_token_usage`** — one row per bearer-authenticated request.
  This is the execution log an operator drills into when debugging a
  specific MCP client, agent, or scripted integration.
- **`api_token_usage_daily`** — nightly rollup keyed by `(token_id,
  day)`. Powers the KPI dashboard and long-window charts without
  scanning the raw log.

The purpose mirrors [`docs/domain/provider_call.md`](domain/provider_call.md):
give operators a full wire trace of who is calling the API, from where,
with what payload, and what the server returned — cheap enough to leave
on by default.

## How the middleware captures

The bearer auth middleware (see `internal/infrastructure/http/middleware/`)
sits between the router and the handler. On a bearer-authenticated
request it:

1. **Buffers the request body** into a `bytes.Buffer` capped at 8 KiB.
   Bytes beyond the cap are counted but discarded. The original
   `r.Body` is then replaced with an `io.MultiReader` so the downstream
   handler still sees the full stream.
2. **Wraps `ResponseWriter`** in a shim that records the final
   `StatusCode` and mirrors the first 8 KiB of the response body into
   a second capped buffer. `Flush`, `Hijack`, and `Push` are proxied
   through unchanged.
3. **Times the request** with `time.Since(start)` measured across the
   downstream handler + response flush.
4. **Extracts the remote IP** honouring the trusted-proxy list
   (`X-Forwarded-For` first entry when the peer is a configured proxy;
   `RemoteAddr` otherwise).
5. **Redacts** each buffered body: if the payload parses as JSON, the
   keys `password`, `access_token`, `app_secret`, `verify_token`,
   `secrets`, `plaintext`, `token`, `secret` (at any depth) are
   replaced with the string `"[redacted]"`. Non-JSON payloads pass
   through untouched (still capped). Media / binary responses
   (content-type outside the JSON / text / form families) store
   `response_body: null` and record only the byte size.
6. **Persists on a detached goroutine.** After the response is
   flushed to the client, the middleware spawns a bookkeeping
   goroutine (through the shared worker pool in
   `internal/workers/pool.go` — never a raw `go func()`) that inserts
   the row. A MySQL outage logs a warning and swallows the error; the
   client-facing request has already completed.

The detached-goroutine pattern is the same invariant as
`providercall.Service.Record`: **bookkeeping never blocks or fails the
underlying request.**

## Data model

### `api_token_usage`

| Column | Type | Notes |
|---|---|---|
| `id` | `BIGINT UNSIGNED` | Auto-increment. |
| `org_id` | `CHAR(26)` | Tenant boundary. First column of every non-primary index. |
| `token_id` | `CHAR(26)` | FK to `api_tokens.id`. |
| `occurred_at` | `DATETIME(3)` | UTC timestamp when the middleware started timing. |
| `remote_ip` | `VARCHAR(45)` | IPv4 or IPv6, proxy-aware. |
| `method` | `VARCHAR(8)` | HTTP verb. |
| `path` | `VARCHAR(255)` | Route template (`/api/v1/conversations/{id}/messages`), not the substituted URL — bounds cardinality and keeps top-path grouping meaningful. |
| `status_code` | `SMALLINT` | Final HTTP status. |
| `latency_ms` | `INT` | Wall-clock request duration. |
| `request_body` | `BLOB` | Capped at 8 KiB, redacted. May be `NULL` when the request had no body. |
| `request_body_size` | `INT` | Original size in bytes (pre-cap). |
| `response_body` | `BLOB` | Capped at 8 KiB, redacted. `NULL` for media/binary. |
| `response_body_size` | `INT` | Original size in bytes (pre-cap). |

Indexes:

- `PRIMARY (id)`
- `(org_id, token_id, occurred_at DESC)` — powers the paginated log query.
- `(org_id, occurred_at)` — powers the retention worker's range delete.

### `api_token_usage_daily`

| Column | Type | Notes |
|---|---|---|
| `org_id` | `CHAR(26)` | |
| `token_id` | `CHAR(26)` | |
| `day` | `DATE` | UTC calendar day. |
| `call_count` | `BIGINT` | |
| `error_count` | `BIGINT` | Rows with `status_code >= 400`. |
| `latency_p50_ms` | `INT` | |
| `latency_p95_ms` | `INT` | |
| `top_paths` | `JSON` | Small histogram: `[{path, count}, ...]` capped at N entries. |

`PRIMARY KEY (org_id, token_id, day)`. Upserted by a nightly job that
scans the previous UTC day's `api_token_usage`.

## Retention

**Currently unlimited.** Every bearer-authenticated request produces a
row and rows are never pruned.

**Phase 4 TODO** — add a rolling-delete worker (mirrors the pattern
described in [`docs/domain/provider_call.md`](domain/provider_call.md#retention))
that drops `api_token_usage` rows older than 30 days by default,
operator-configurable, while keeping the daily rollup indefinitely.
The `(org_id, occurred_at)` index makes range deletes cheap.

Until that worker ships, operators should:

- Monitor table size via Prometheus (`nudgeway_api_token_usage_row_count` — TODO).
- Periodically run `DELETE FROM api_token_usage WHERE occurred_at < NOW() - INTERVAL 30 DAY` from `nudgeway-cli` (TODO subcommand) if the table grows uncomfortably large.

## Operator surfaces

### UI

`/settings/api-tokens` lists every token in the org. Clicking a row
opens a right-side drawer with two tabs:

- **Overview** — KPI cards (call count, error rate, p50 / p95 latency),
  a per-day sparkline of the selected window, and a top-paths table.
  Data source: `GET /api/v1/api-tokens/{id}/metrics`.
- **Log** — reverse-chronological table of individual requests with
  filters for method, status class, and free-text path substring. Each
  row expands to show the redacted request + response bodies. Data
  source: `GET /api/v1/api-tokens/{id}/usage` (cursor-paginated).

### REST

```
GET /api/v1/api-tokens/{id}/usage?from={rfc3339}&to={rfc3339}&limit={n}&cursor={opaque}
GET /api/v1/api-tokens/{id}/metrics?from={rfc3339}&to={rfc3339}
```

Both are guarded by the `api_tokens.read` RBAC scope. Users can always
read their own token's usage; org admins can read any token in the
org. Cross-tenant reads are rejected at the query layer (see CLAUDE.md
§2 prime directive #5).

### MCP tools

The two endpoints surface automatically as MCP tools via the OpenAPI
→ tool generator described in [`docs/mcp.md`](mcp.md):

- `getAPITokenUsage` — paginated execution log.
- `getAPITokenMetrics` — KPIs + per-day series + top paths.

See [`skills/nudgeway-api-tokens/SKILL.md`](../skills/nudgeway-api-tokens/SKILL.md#common-patterns)
for the "last 24 hours" tool-call recipe.

## Related

- [`skills/nudgeway-api-tokens/SKILL.md`](../skills/nudgeway-api-tokens/SKILL.md) — token mint / list / revoke / read-usage skill.
- [`docs/domain/provider_call.md`](domain/provider_call.md) — sister log for outbound provider HTTP; same detached-goroutine + capped-body pattern.
- [`docs/mcp.md`](mcp.md) — MCP server + tool surface.
- CLAUDE.md §15 (security rules) and §16 (observability rules) — the invariants this feature implements.
