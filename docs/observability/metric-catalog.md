# Nudgeway metric catalog

Every metric emitted by the Nudgeway binary. Registered onto the dedicated
`*prometheus.Registry` owned by `internal/infrastructure/metrics.Metrics`
and served from `GET /metrics` in the standard Prometheus /
OpenMetrics exposition format.

Naming convention (enforced by review): `nudgeway_<subsystem>_<name>_<unit>`.
Never use raw provider names as label values that could explode
cardinality — stick to the fixed labels documented here.

## HTTP

### `nudgeway_http_requests_total`

- **Kind:** counter
- **Labels:** `method` (uppercase HTTP verb), `path` (route template — never the raw URL), `status` (numeric HTTP status code as a string)
- **Unit:** requests
- **Trigger:** incremented once per request served by any handler wrapped with `metrics.Metrics.HTTPMiddleware(route)`.

### `nudgeway_http_request_duration_seconds`

- **Kind:** histogram (default Prometheus buckets)
- **Labels:** `method`, `path`, `status`
- **Unit:** seconds
- **Trigger:** observed once per request served by the middleware; measured from handler entry to handler return.

## Providers

### `nudgeway_provider_calls_total`

- **Kind:** counter
- **Labels:** `provider` (e.g. `whatsapp`, `zoho_desk`), `operation` (e.g. `send_message`, `download_media`), `outcome` (`ok`, `error`, `rate_limited`, `auth`)
- **Unit:** calls
- **Trigger:** every outbound call to a third-party API from an adapter under `internal/providers/*`. Recorded after the call completes (success or failure).

### `nudgeway_provider_call_duration_seconds`

- **Kind:** histogram (default Prometheus buckets)
- **Labels:** `provider`, `operation`, `outcome`
- **Unit:** seconds
- **Trigger:** observed for every provider call, wall-clock from dispatch to completion (including retries — a single logical call is one observation).

## Workers

### `nudgeway_worker_jobs_total`

- **Kind:** counter
- **Labels:** `lane` (e.g. `message.send`, `webhook.process`, `campaign.job`), `group` (Redis Streams / Kafka consumer group), `outcome` (`ok`, `error`, `dead_letter`)
- **Unit:** jobs
- **Trigger:** incremented once per job the worker pool finishes processing.

### `nudgeway_worker_job_duration_seconds`

- **Kind:** histogram (default Prometheus buckets)
- **Labels:** `lane`, `group`, `outcome`
- **Unit:** seconds
- **Trigger:** observed per job — wall-clock from `handler.Handle` entry to return.

### `nudgeway_worker_job_retries_total`

- **Kind:** counter
- **Labels:** `lane`, `group`
- **Unit:** retry attempts
- **Trigger:** incremented every time the worker pool re-queues a job after a transient failure. Terminal `dead_letter` is a job outcome, not a retry.

## Webhooks

### `nudgeway_webhook_events_received_total`

- **Kind:** counter
- **Labels:** `provider`, `integration_id` (opaque tenant integration id — cardinality caps once every tenant integration is stable; if this becomes a problem, hash + truncate)
- **Unit:** events
- **Trigger:** incremented by the webhook ingress after signature verification and idempotency insert, before enqueue.

## Kafka

### `nudgeway_kafka_producer_batch_bytes_total`

- **Kind:** counter
- **Labels:** `topic`
- **Unit:** bytes
- **Trigger:** the Kafka producer records the batch size (serialized payload bytes) on every successful produce ack.

### `nudgeway_kafka_consumer_lag_records`

- **Kind:** gauge
- **Labels:** `topic`, `partition`, `group`
- **Unit:** records
- **Trigger:** the consumer refreshes this gauge on each poll cycle using the difference between the log end offset and the group's committed offset.

## WebSocket

### `nudgeway_websocket_connections`

- **Kind:** gauge
- **Labels:** `org_id_short` — first 8 hex chars of the org id (bounded cardinality; still lets us alert per-tenant on abandonment or storms)
- **Unit:** connections
- **Trigger:** `Inc()` on `Connect`, `Dec()` on `Disconnect` (including abnormal close).

## Runtime + process (free)

Registered alongside the Nudgeway families:

- `go_*` — provided by `prometheus.NewGoCollector()` — goroutines, GC pauses, heap, stack, and Go build info.
- `process_*` — provided by `prometheus.NewProcessCollector()` — RSS, open FDs, CPU seconds, start time.

## Notes

- **No labels for `org_id` directly.** High-cardinality labels destroy Prometheus. Use `org_id_short` (first 8 hex chars) where per-tenant visibility matters, otherwise attach `org_id` on the trace / log side.
- **`outcome` is a closed enum.** Adding a value is a documentation change — bump the catalog in the same commit.
- **Handler wraps recover routing.** The HTTP middleware relies on the `path` argument coming from the router, not the URL — so 404s land under a stable `not_found` route rather than exploding cardinality by raw path.
