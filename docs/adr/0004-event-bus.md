# ADR 0004 — Internal event bus

Status: Accepted (2026-09-03)

## Context

The platform is async-first. Every feature ends with a canonical event that fans out to WebSocket, automation, analytics, ticket sync, and other subscribers. We need in-proc fan-out for same-node handlers and durable fan-out for cross-node delivery + worker consumption.

## Decision

- Two implementations of the `eventbus.Publisher` port:
  - `internal/events/inproc.go` — synchronous, in-process, direct dispatch. Used for lightweight subscribers (WebSocket hub, cache invalidators).
  - `internal/infrastructure/redis/streams.go` (Phase 1) — durable Redis Streams with per-lane consumer groups.
- Events are strongly typed (`internal/domain/events.Envelope`); wire format is protobuf.
- Ordering: per-`conversation_id` ordering preserved by consumer-group key.

## Consequences

- Same event API for local and cross-node handlers.
- We can migrate individual event types from in-proc to streams (or vice versa) without changing publishers.
- Ordering guarantees are per-conversation, not global — designed-for, not stumbled into.

---

**Superseded by [ADR 0009](0009-kafka-for-event-log.md) for the durable path (2026-09-04).**

The Redis Streams implementation described above was a Phase 0 placeholder. Phase 1 replaces the durable path with Kafka via `internal/infrastructure/kafka`. The in-process bus (`internal/events/inproc.go`) is unchanged. The `queue.Enqueuer` / `queue.Consumer` and `eventbus.Publisher` / `eventbus.Subscriber` ports are unchanged — only the implementation moved.
