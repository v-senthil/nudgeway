# ADR 0003 — Storage: MySQL + Redis + HBase

Status: Accepted (2026-09-03)

## Context

Three distinct storage workloads:
1. Small, relational, must be transactional: orgs, users, roles, contacts, sessions, conversations, tickets, integrations.
2. Ephemeral coordination: queues, locks, cache, rate limits, idempotency, presence.
3. Very high-volume append-only: message payloads, webhook raw events, activity streams, analytics, attachments.

## Decision

- **MySQL 8+** for (1). Source of truth. Every table `org_id`-scoped indexes.
- **Redis 7+** for (2). Streams for durable queues; `SET NX PX` for locks; standard cache patterns.
- **HBase 2+** for (3). Tenant-prefixed, time-bucketed row keys — no cross-tenant scans, ever.

All three run natively on the developer's machine; no Docker.

## Consequences

- We swim in familiar waters — MySQL semantics are boring in a good way.
- HBase row-key design is a load-bearing skill; documented at `docs/architecture.md`.
- Adding a new large-scale query pattern requires a design pass on row keys, not just an index.
