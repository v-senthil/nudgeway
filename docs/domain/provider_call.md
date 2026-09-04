# Domain — ProviderCall

The `providercall` domain package models one entry in the operator-facing
execution log — the record of a single outbound HTTP call a provider adapter
made to a third-party API.

## Purpose

Debugging real-world provider integrations is unreasonably hard without an
authoritative record of exactly what we sent, exactly what came back, and
how long it took. The Meta Graph API in particular fails in strange,
under-documented ways (error code 100 subcode 33 GraphMethodException,
templates rejected without a body, media downloads that 401 five seconds
after a `get_media_url` succeeds). Grep-based log spelunking is not enough.

`provider_calls` gives operators:

- Per-integration + per-operation drill-down (all `send_message` failures
  from integration X in the last hour).
- Full request body + response body for every attempt, so a Meta support
  ticket can be filed with the exact wire trace.
- Meta's `fbtrace_id` on every entry — the single required datum for
  filing a Meta case.
- Latency histograms without needing Prometheus.

## Types

### `providercall.Entry`

| Field | Type | Notes |
|-------|------|-------|
| `ID` | `uint64` | Auto-increment. Zero on unpersisted entries. |
| `OrgID` | `string` | Tenant boundary. Required. |
| `IntegrationID` | `string` | May be empty on very-early failures. |
| `Provider` | `string` | Registry key (`whatsapp`, ...). Required. |
| `Operation` | `string` | Adapter method — see the `Op*` constants. |
| `Direction` | `providercall.Direction` | `outbound` today; `inbound` reserved. |
| `Method` | `string` | HTTP verb. |
| `URL` | `string` | Fully-qualified URL — no secrets. |
| `StatusCode` | `int` | Zero when the request never completed. |
| `LatencyMs` | `int64` | Wall-clock request duration. |
| `RequestBody` | `[]byte` | Truncated at `MaxBodyBytes` (default 64 KiB). |
| `ResponseBody` | `[]byte` | Same. Nil for `download_media`. |
| `ErrorClass` | `string` | `transient` \| `rate_limited` \| `auth` \| `permanent` \| `unknown` \| `""`. |
| `ErrorMessage` | `string` | Truncated at 1024 chars. |
| `TraceID` | `string` | Meta's `fbtrace_id`. |
| `CorrelationID` | `string` | Stitches back to the originating request / job. |
| `OccurredAt` | `time.Time` | UTC. |

### `providercall.Operation`

Constants:

| Constant | Value | Emitted by |
|----------|-------|------------|
| `OpSendMessage` | `send_message` | `Provider.SendMessage` → `client.sendMessage` |
| `OpMarkAsRead` | `mark_as_read` | `Provider.MarkAsRead` → `client.markAsRead` |
| `OpGetMediaURL` | `get_media_url` | `Provider.DownloadMedia` → `client.getMediaURL` |
| `OpDownloadMedia` | `download_media` | `Provider.DownloadMedia` / `DownloadMediaByURL` → `client.downloadMedia` |
| `OpListTemplates` | `list_templates` | `client.listTemplates` |
| `OpCreateTemplate` | `create_template` | `client.createTemplate` |
| `OpGetTemplateStatus` | `get_template_status` | `client.getTemplateStatus` |
| `OpUploadMedia` | `upload_media` | `Provider.UploadMedia` → `client.uploadMedia` |

The persisted `operation` column is a free-form `VARCHAR(64)` — a new
adapter may define its own operation strings without a schema change.

## Invariants

1. **No secret headers.** `Authorization: Bearer <token>` MUST NOT land in
   the table. The tracer at `internal/providers/whatsapp/tracer.go`
   deliberately has no headers field on `TraceEvent`; if a future refactor
   adds one, `Entry.Redact` grows scrubbing logic and callers wrap
   defensively.

2. **Never blocks the caller.** `Service.Record` is fire-and-forget. A
   downed MySQL logs a warning and swallows the error. The outbound HTTP
   call the entry describes always succeeds or fails on its own merits —
   never because of bookkeeping.

3. **Bodies truncated at `MaxBodyBytes`.** Default 64 KiB. Set
   `Deps.MaxBodyBytes` to override. Truncation is safe: Meta error
   envelopes with embedded diagnostics fit in a few KiB.

4. **`download_media` never records the response body.** The response body
   for that operation is the media itself — potentially megabytes of
   binary, useless for debugging.

5. **`upload_media` never records the raw multipart body.** The tracer
   substitutes a synthetic summary (`{filename, content_type, size}`).
   Same reason.

6. **Every write is org-scoped.** `Record` refuses entries with empty
   `OrgID`. `List` requires an `orgID` argument; there is no cross-tenant
   read surface.

7. **Append-only.** There is no update path. See "Retention" below.

## Retention

Meta send lanes can produce thousands of rows per day on an active tenant.
A Phase 4 rolling-delete job will drop rows older than 30 days by default
(operator-configurable). The composite `(org_id, occurred_at)` index makes
range deletes cheap.

Until that job lands, operators should monitor `provider_calls` table
size via Prometheus (`fullwa_provider_calls_row_count` — TODO) and
periodically run `DELETE FROM provider_calls WHERE occurred_at < NOW() -
INTERVAL 30 DAY` from `fullwa-cli` (TODO subcommand).

## Related

- Flow: [`docs/flows/provider-call-recording.md`](../flows/provider-call-recording.md).
- Provider integration: [`docs/providers/whatsapp.md`](../providers/whatsapp.md#observability--meta-api-execution-logs).
