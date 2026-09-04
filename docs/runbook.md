# Nudgeway runbook

Operational procedures for the Nudgeway platform. Expands per phase — Phase 0 lands the skeleton.

## Local dev bring-up

1. Start MySQL, Redis, HBase (all natively — `brew services start mysql redis`; HBase via `start-hbase.sh`).
2. `make check-infra` — verifies reachability.
3. `make migrate up` — applies schema.
4. `make dev` — runs backend + frontend.

## Health probes

- `GET /healthz` — 200 if the process is alive.
- `GET /readyz` — 200 only if MySQL, Redis, HBase, and Kafka are reachable. 503 otherwise; caller should retry.
  - Kafka probe (`internal/infrastructure/health/kafka.go`) TCP-dials each broker with a 500 ms timeout; green if any broker answers. A deeper metadata probe lands in Phase 2.

## Metrics

- `GET /metrics` — Prometheus / OpenMetrics exposition served from the Nudgeway registry (`internal/infrastructure/metrics`).
- All metric names follow `nudgeway_<subsystem>_<name>_<unit>`. The full enumeration — labels, unit, and what triggers each — lives in [`docs/observability/metric-catalog.md`](observability/metric-catalog.md).
- Families emitted:
  - **HTTP** — `nudgeway_http_requests_total`, `nudgeway_http_request_duration_seconds` (labels: method, path, status). Recorded by `metrics.Metrics.HTTPMiddleware`.
  - **Providers** — `nudgeway_provider_calls_total`, `nudgeway_provider_call_duration_seconds` (labels: provider, operation, outcome). Recorded inside each provider adapter under `internal/providers/*`.
  - **Workers** — `nudgeway_worker_jobs_total`, `nudgeway_worker_job_duration_seconds`, `nudgeway_worker_job_retries_total` (labels: lane, group, outcome). Recorded by the worker pool.
  - **Webhooks** — `nudgeway_webhook_events_received_total` (labels: provider, integration_id). Incremented by the webhook ingress.
  - **Kafka** — `nudgeway_kafka_producer_batch_bytes_total` (labels: topic), `nudgeway_kafka_consumer_lag_records` (labels: topic, partition, group). Recorded by the Kafka producer/consumer.
  - **WebSocket** — `nudgeway_websocket_connections` (label: org_id_short). Tracked by the WS server around Connect/Disconnect.
- Standard `GoCollector` + `ProcessCollector` are also registered — Go runtime and process stats come for free.
- Scrape config: point Prometheus at `http://<host>:8080/metrics`. No authentication in local; expose behind an internal-only listener in prod.

## Runbook items (added per feature)

- (Phase 0 Task 2) Rotating the credential KEK.
- (Phase 1) Replaying a stuck webhook event.
- (Phase 1) Draining and restarting a worker pool.
- (Phase 2) Recovering a `PENDING_SYNC` ticket after a provider outage.
- (Phase 3) Pausing / resuming a campaign.
- (Phase 4) Backup + restore procedures (MySQL logical dump + HBase snapshot).
