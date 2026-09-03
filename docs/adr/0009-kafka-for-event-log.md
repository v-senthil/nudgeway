# ADR 0009 — Kafka for the durable event log and job queues

Status: Accepted (2026-09-04)

## Context

ADR 0004 sketched the internal event bus with two implementations:

- `internal/events/inproc.go` — synchronous, in-process fan-out.
- `internal/infrastructure/redis/streams.go` — durable Redis Streams for cross-node fan-out + job queues.

The Redis Streams path was a placeholder chosen for zero extra infra during Phase 0. As Phase 1 begins, real background workers need a durable log with:

- per-`conversation_id` ordering across nodes,
- consumer groups with independent progress per subscriber,
- durable replay for debugging + backfill,
- backpressure that a job producer can rely on,
- headroom for higher-volume traffic in later phases (campaigns, analytics fan-out).

Kafka is available natively on the developer machine, matches the "no Docker / no Kubernetes" preference, and covers every property above without inventing our own consumer-group semantics on Streams.

## Decision

Adopt Apache Kafka as the durable event log **and** job queue backend behind the existing `queue` and `eventbus` ports.

- Library: [`github.com/twmb/franz-go`](https://github.com/twmb/franz-go) — pure Go, no CGO, actively maintained, high performance.
- Topic naming:
  - `<prefix>.jobs.<lane>` — one topic per background job lane.
  - `<prefix>.events.<type>` — one topic per canonical event type.
- Partition key:
  - Jobs: `queue.Job.ID`. Upstream code MUST set `Job.ID` to a value that is either the `conversation_id` (message flows) or a deterministic derivation of it. This preserves per-conversation ordering inside one partition.
  - Events: `events.Envelope.CorrelationID`. For message-flow events upstream already sets this to the conversation id.
- Producer: idempotent, `acks=all`, snappy compression, 5 ms linger. Synchronous produce so callers see broker errors on return.
- Consumer: one franz-go client per lane per group. Auto-commit disabled — records are committed only after the handler returns nil. On error, the record is redelivered on next poll; retry / backoff / DLQ live in the caller's job envelope, not in the transport.
- Wire format: JSON for now. A protobuf schema is a follow-up — see TODO in `docs/phases/phase-1.md`.

Redis stays in the picture for what it is actually good at: cache, distributed locks, rate limiters, idempotency keys, WebSocket presence, and short-lived state. It is no longer on the critical path for durable event delivery.

The in-process bus (`internal/events/inproc.go`) is unchanged and remains the fastest path for same-node subscribers (WebSocket hub, cache invalidators). Application code chooses which bus to publish to when wiring services; both satisfy `eventbus.Publisher`.

## Consequences

**Gains**

- Durable, replayable event log with real consumer groups.
- Per-partition ordering by key gives per-conversation ordering by construction.
- Better operator tooling: `kafka-topics`, `kafka-console-consumer`, ecosystem UIs.
- The `queue.Job.Attempt` field can now grow into a real retry policy without racing on Redis Streams `XCLAIM`.

**Costs**

- One more infra dependency to run locally. Documented in `CLAUDE.md §5`.
- Slightly higher end-to-end latency vs pure Redis Streams for tiny payloads — mitigated by 5 ms linger + snappy compression.
- Topic explosion if event types proliferate. Mitigated by the `<prefix>.events.<type>` naming and by treating rare event types as candidates for consolidation.

**Open items**

- Protobuf wire format (JSON is Phase 1 baseline).
- Consumer-group naming convention for multi-tenant fan-out.
- Managed retry / DLQ topics — likely `<prefix>.jobs.<lane>.retry` and `<prefix>.jobs.<lane>.dlq`.
- HBase archival consumer that copies every event topic into `webhook_events` / `activity` for long-term replay.
