# Flow — Provider-call recording

Every outbound HTTP call the WhatsApp adapter makes to Meta lands in the
`provider_calls` table for operator debugging. Recording is fire-and-forget:
a downed MySQL logs a warning and swallows the error — the outbound HTTP
call the entry describes always succeeds or fails on its own merits.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant App as Application layer<br/>(SendService / ReadService / ...)
    participant Prov as whatsapp.Provider
    participant Client as whatsapp.client
    participant Meta as Meta Graph API
    participant Tracer as Tracer<br/>(closure over providercall.Service)
    participant Svc as providercall.Service
    participant Repo as mysql.ProviderCalls
    participant DB as MySQL provider_calls

    App->>Prov: SendMessage(ctx, req)
    Prov->>Client: sendMessage(ctx, body)
    Client->>Client: doJSON(ctx, "send_message", POST, url, body)
    loop up to 4 attempts on retryable errors
        Client->>Client: doOnce — start := time.Now()
        Client->>Meta: HTTP request<br/>(Authorization: Bearer …)
        Meta-->>Client: HTTP response<br/>(status, body, x-fb-trace-id)
        Client->>Client: raw := io.ReadAll(res.Body, 8 MiB cap)
        Client->>Tracer: OnCall(ctx, TraceEvent{op, method, url,<br/>request_body, response_body, status_code,<br/>latency_ms, err_class, err_message, trace_id,<br/>integration_id, org_id})
        Tracer->>Svc: Record(ctx, providercall.Entry{...})
        Svc->>Svc: truncate bodies to MaxBodyBytes (64 KiB)
        Svc->>Svc: Redact() — no-op today
        Svc->>Repo: Record(ctx, entry)
        Repo->>DB: INSERT INTO provider_calls (...)
        DB-->>Repo: LAST_INSERT_ID
        Repo-->>Svc: id, nil
        Svc-->>Tracer: (fire-and-forget; errors logged not returned)
        Tracer-->>Client: return
        alt status < 400
            Client->>Client: decode JSON into out
            Client-->>Prov: nil
        else 4xx / 5xx
            Client->>Client: parseErrorResponse → APIError
            Client-->>Prov: APIError (Retryable? then loop)
        end
    end
    Prov-->>App: SendResult{ProviderMessageID, AcceptedAt}
```

## Failure modes

| Failure | What happens |
|---------|--------------|
| MySQL down when `Record` runs | Warning logged with `provider`, `operation`, `org_id`, `err`. Entry lost. Outbound HTTP call unaffected. |
| Tracer panics | Recovered inside `client.trace` (defer). Log line lost. Outbound HTTP call unaffected. |
| Request body larger than `MaxBodyBytes` | Truncated to `MaxBodyBytes` before persist. Length in the log line marks it as truncated (future improvement). |
| `download_media` succeeds | Entry recorded with `status_code=200`, `latency_ms`, `trace_id`. Response body intentionally empty — raw bytes are the media itself. |
| `upload_media` succeeds | Entry recorded with a synthetic request body `{filename, content_type, size}` instead of the raw multipart bytes. |
| Retry loop fires N times | Each attempt emits its own `TraceEvent`. Operator sees the full retry history newest-first when they filter by `correlation_id`. |

## Read path

Operators pull the log via `GET /api/v1/provider-calls` (see the OpenAPI
spec + `internal/api/rest/v1/provider_calls.go`). The frontend viewer is at
`/settings/provider-calls`. Filters: integration_id, operation, status
range, since / until. Cursor-paginated; opaque base64 tokens.

## Wire-up (informational)

`cmd/server/main.go` (follow-up commit) will:

1. Construct `mysql.ProviderCalls` against the shared `*sql.DB`.
2. Construct `providercall.Service` with `Deps{Repo, Logger}`.
3. Construct a `Tracer` closure that calls `svc.Record` with the
   `TraceEvent` mapped to `providercall.Entry` (setting `Provider =
   "whatsapp"`).
4. Chain `.WithTracer(tracer, integration.ID, orgID)` on the
   `whatsapp.Provider` construction — one Provider per integration row.
5. Thread the service into `v1.Deps.ProviderCalls` so the REST route
   mounts.
