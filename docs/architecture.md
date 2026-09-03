# fullWA architecture

## Overview

fullWA is a **modular monolith** written in Go, compiled to a single binary that hosts the REST API, WebSocket server, webhook ingress, background workers, and scheduler. The frontend is a React SPA embedded via `//go:embed`.

Three infrastructure services, all running natively on the developer's machine (no Docker):

| Store | Role |
|-------|------|
| **MySQL** | Transactional source of truth. Organizations, users, contacts, sessions, conversations, message metadata, tickets, templates, campaigns, automation, integrations, audit logs. |
| **Redis** | Coordination + speed. Job queues (Streams), distributed locks, cache, rate limiters, idempotency keys, WebSocket presence. |
| **HBase** | High-volume append-only. Message payloads, webhook raw events, activity streams, analytics events, attachments. |

## Prime directives

1. **Canonical domain → Persist → Event → Async processing → Provider adapter → Result → Event → Real-time UI.**
2. Contact ≠ Session ≠ Conversation ≠ Message ≠ Ticket — all first-class.
3. Never hold a DB transaction while making an external API call.
4. No provider-specific types in `internal/domain/*` or `internal/application/*`.
5. Multi-tenancy enforced at every query layer.
6. Idempotency on every webhook + every outbound send.
7. Persist authoritative state before async processing.
8. Single binary; modular monolith.

## Dependency rule

```
domain          → stdlib + domain siblings only
application     → domain + ports
infrastructure  → ports (implements them) + drivers
providers       → ports (implements them) + provider SDKs
cmd             → wires everything together
```

Enforced by `.go-arch-lint.yml` in CI.

## Request → response lifecycle

```
                  ┌───────────────────────────────┐
                  │        HTTP / WebSocket        │
                  └────────────┬──────────────────┘
                               │ chi router + middleware
                               ▼
                  ┌───────────────────────────────┐
                  │   internal/api/rest/*         │
                  │  (generated from openapi.yaml)│
                  └────────────┬──────────────────┘
                               │
                               ▼
                  ┌───────────────────────────────┐
                  │  internal/application/*       │
                  │  (use-case orchestration)     │
                  └───────┬───────────┬───────────┘
                          │           │
                          ▼           ▼
              ┌─────────────────┐ ┌────────────────────┐
              │ internal/ports/  │ │  internal/domain/ │
              │ (interfaces)    │ │ (pure Go entities)│
              └───────┬─────────┘ └────────────────────┘
                      │
      ┌───────────────┴───────────────┐
      ▼                               ▼
┌──────────────────┐          ┌──────────────────┐
│ infrastructure/  │          │ providers/       │
│  MySQL, Redis,   │          │  WhatsApp, Zoho, │
│  HBase, WS, auth │          │  OpenAI, …       │
└──────────────────┘          └──────────────────┘
```

## Async spine

```
                         canonical event
                                │
                ┌───────────────┼───────────────┐
                ▼               ▼               ▼
        WebSocket hub     Automation      Redis Streams
         (real-time UI)      engine        (workers, other nodes)
                                                │
                              ┌─────────────────┼──────────────────┐
                              ▼                 ▼                  ▼
                       Message send      Ticket sync         AI invoke
                       worker            worker              worker
                              │                 │                  │
                              ▼                 ▼                  ▼
                     WhatsApp adapter  Zoho Desk adapter    OpenAI adapter
```

Persist authoritative state first, then publish. Retries + idempotency at every external hop.

## Data architecture

### MySQL

Every table carries `org_id` as the first column of every non-primary index. One `ACTIVE` Session per `(org, business_endpoint, contact)` enforced with a partial unique index. `WebhookEvent(integration_id, external_event_id)` unique for idempotent webhook ingestion.

### Redis

- **Queues** — one Stream per lane (`q:message.send`, `q:webhook.process`, `q:campaign.job`, `q:ticket.sync`, `q:ai.invoke`). Consumer groups per worker pool.
- **Locks** — `SET NX PX` with fencing tokens.
- **Cache** — templates, integration configs, capabilities, user permissions.
- **Rate limits** — leaky bucket per (org, integration, endpoint).
- **Idempotency** — `SET NX EX` keyed on `external_event_id` / `Idempotency-Key`.

### HBase

| Table | Row key | Access pattern |
|-------|---------|-----------------|
| `messages` | `<org_short>|<conversation_id>|<reverse_ts>|<message_id>` | conversation thread newest-first |
| `messages_by_contact` | `<org_short>|<contact_id>|<reverse_ts>|<message_id>` | contact 360 timeline |
| `webhook_events` | `<org_short>|<yyyymmddhh>|<integration_id>|<event_id>` | replay / debug |
| `activity` | `<org_short>|<contact_id>|<reverse_ts>|<event_id>` | activity stream |
| `analytics_events` | `<org_short>|<yyyymmdd>|<metric>|<dim_hash>|<uuid>` | rollups |
| `attachments` | `<sha256_prefix>|<sha256>` | content-addressed dedupe |

## Provider adapter pattern

Each provider is a package under `internal/providers/<name>/`. It implements one or more port interfaces from `internal/ports/{channel,ticketing,bot,aiport,calling}/`. Providers self-register via `init()` into `internal/providers/registry.go`. Nothing outside `internal/providers/*` imports a provider package by name.

To add a new provider, see [`CLAUDE.md` §7](../CLAUDE.md).

## OpenAPI-first

`internal/api/openapi/openapi.yaml` is the source of truth for every REST endpoint. Go server interfaces are generated via `oapi-codegen`; the TS client is generated via `openapi-typescript` + `openapi-fetch`. Errors are RFC 7807 (`application/problem+json`). Spectral + `openapi-diff` gate the spec in CI.

## Real-time

WebSocket hub per node, coordinated via Redis pub/sub. Per-org rooms; per-conversation subscriptions. Presence + typing tracked in Redis with short TTL.

## Observability

- Structured JSON logs via `log/slog`.
- Prometheus metrics on `/metrics`.
- OpenTelemetry traces via OTLP.
- `request_id` on every request, `correlation_id` propagated to every job, `causation_id` on every event.

See [`docs/adr/`](adr/) for architectural decisions and [`docs/phases/`](phases/) for delivery status.
