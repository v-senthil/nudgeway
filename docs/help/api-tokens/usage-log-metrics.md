# Usage log + metrics

Every request that authenticates via `Authorization: Bearer <token>` is captured in a per-token execution log plus a nightly rollup. This is the surface an operator drills into when debugging a specific MCP client, agent, or scripted integration.

## How to use

1. Open **Settings → API tokens** (`/settings/api-tokens`).
2. Click any token row. A right-side drawer opens with two tabs.

### Overview tab

- **KPI cards** — call count, error rate, p50 / p95 latency for the selected window.
- **Per-day sparkline** — call volume over the window.
- **Top paths** — request-path histogram (route template, not the substituted URL).

Data source: `GET /api/v1/api-tokens/{id}/metrics`.

### Log tab

- Reverse-chronological table: `occurred_at`, `remote_ip`, `method`, `path`, `status_code`, `latency_ms`.
- Filters: HTTP method, status class (`2xx` / `4xx` / `5xx`), free-text path substring.
- Each row expands to show the redacted request + response bodies.

Data source: `GET /api/v1/api-tokens/{id}/usage` (cursor-paginated).

## How the middleware captures

The bearer auth middleware sits between the router and the handler. For every bearer-authenticated request it:

1. **Buffers the request body** into a buffer capped at **8 KiB**. Bytes beyond the cap are counted but discarded; the downstream handler still sees the full stream via an `io.MultiReader`.
2. **Wraps `ResponseWriter`** to capture the final `StatusCode` and mirror the first 8 KiB of the response body.
3. **Times the request** with `time.Since(start)`.
4. **Extracts the remote IP**, honouring the trusted-proxy list (`X-Forwarded-For` first entry when the peer is a configured proxy; `RemoteAddr` otherwise).
5. **Redacts** each buffered body. If the payload parses as JSON, the keys `password`, `access_token`, `app_secret`, `verify_token`, `secrets`, `plaintext`, `token`, `secret` (at any depth) are replaced with `"[redacted]"`. Non-JSON payloads pass through untouched (still capped). Media / binary responses store `response_body: null` and record only the byte size.
6. **Persists on a detached goroutine** through the shared worker pool. A MySQL outage logs a warning and swallows the error; the client-facing request has already completed. Bookkeeping never blocks or fails the underlying request.

## Data model

- `api_token_usage` — one row per bearer-authenticated request. Indexes: `(org_id, token_id, occurred_at DESC)` powers the paginated log; `(org_id, occurred_at)` powers future retention pruning.
- `api_token_usage_daily` — nightly rollup keyed by `(org_id, token_id, day)` with `call_count`, `error_count`, `latency_p50_ms`, `latency_p95_ms`, and a small `top_paths` JSON histogram.

Retention is currently **unlimited**. A rolling-delete worker (30-day default, operator-configurable) is scheduled for Phase 4.

## API

```
GET /api/v1/api-tokens/{id}/usage?from={rfc3339}&to={rfc3339}&limit={n}&cursor={opaque}
GET /api/v1/api-tokens/{id}/metrics?from={rfc3339}&to={rfc3339}
```

Both are guarded by the `api_tokens.read` RBAC scope. Users can always read their own token's usage; org admins can read any token in the org. Cross-tenant reads are rejected at the query layer.

## MCP

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

```json
{
  "tool": "getAPITokenMetrics",
  "arguments": {
    "id": "01JC5XYZTOKENID",
    "from": "2026-08-06T00:00:00Z",
    "to":   "2026-09-05T00:00:00Z"
  }
}
```

## Troubleshooting

- **Log rows missing bodies.** The request or response exceeded 8 KiB and was truncated, or the content-type was outside the JSON / text / form families (media / binary). The `request_body_size` / `response_body_size` columns still record the original bytes.
- **Redacted values in the log.** Expected — see the redaction key list above. For full unredacted payloads use the provider-call log or application traces.
- **Metrics tab empty for a fresh token.** The daily rollup runs nightly. Until the first roll executes, the KPI cards read from live `api_token_usage` and may show sparse data.

## Related

- [Overview](/#/api-tokens/overview)
- [Create a token](/#/api-tokens/create-token)
- [Revoke a token](/#/api-tokens/revoke-token)
- [Meta API execution log](/#/audit-telemetry/provider-calls) — sister log for outbound provider HTTP; same detached-goroutine + capped-body pattern.
