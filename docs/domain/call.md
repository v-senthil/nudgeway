# Domain: Call

The `Call` aggregate models a single voice call — inbound or outbound —
between the business and a customer. It is a first-class canonical entity,
peer to `Message`, `Conversation`, and `Ticket`.

## Code references

- Type: [`internal/domain/call/call.go`](../../internal/domain/call/call.go)
- Errors: [`internal/domain/call/errors.go`](../../internal/domain/call/errors.go)
- Port (persistence): [`internal/ports/repository/calls.go`](../../internal/ports/repository/calls.go)
- Port (provider): [`internal/ports/calling/calling.go`](../../internal/ports/calling/calling.go)
- Application service: [`internal/application/call/service.go`](../../internal/application/call/service.go)
- MySQL impl: [`internal/infrastructure/mysql/calls.go`](../../internal/infrastructure/mysql/calls.go)
- Migration: [`migrations/20260904000005_calls.up.sql`](../../migrations/20260904000005_calls.up.sql)
- REST: [`internal/api/rest/v1/calls.go`](../../internal/api/rest/v1/calls.go)
- Events: [`internal/domain/events/call_events.go`](../../internal/domain/events/call_events.go)

## Identity + tenancy invariants

- Every row carries `org_id`. There is no cross-tenant query path.
- `(org_id, provider, provider_call_id)` is unique — webhook redelivery is
  idempotent.
- `ProviderCallID` is provider-native. Meta uses `wacid.*`; other providers
  use their own opaque token.

## State machine

```text
queued ─────► ringing ─────► answered ─────► in_progress ─────► completed
   │              │              │                                  ▲
   │              │              └──► completed                     │
   │              ├──► missed                                       │
   │              ├──► declined                                     │
   │              ├──► no_answer                                    │
   │              └──► failed                                       │
   └──► failed                                                      │
                                                                    │
Terminal statuses (no further transitions):                         │
  completed | missed | failed | declined | no_answer ───────────────┘
```

`call.Status.Terminal()` reports whether a status is terminal.

## Invariants enforced by the schema

- `direction` is one of `inbound` / `outbound`.
- `duration_seconds` never goes down — `AttachRecording` uses
  `GREATEST(duration_seconds, ?)`.
- Time stamps advance monotonically via
  `started_at = COALESCE(started_at, ?)` etc. so a late-arriving webhook
  cannot blank an already-stamped `answered_at`.
- `metadata` is a non-nullable JSON column; the service always writes at
  least `{}` so downstream SQL never has to deal with `NULL`.

## Linkage to other aggregates

- `business_endpoint_id`, `contact_id`, `session_id`, `conversation_id`
  are all nullable. The webhook ingest pipeline backfills them as it
  resolves the linkage — an early inbound webhook whose contact hasn't
  been upserted yet still lands as a row with the linkage columns null.
- `integration_id` is required (non-null) — a call without a resolvable
  integration cannot be recorded because we can't attribute its costs.

## Recording + transcription

- `recording_url` is either a Meta short-lived URL (before background
  downloader lands) or a self-hosted `/api/v1/media/<sha>` key.
- `transcription_ref` is the provider media asset id for the transcript
  JSON document. The download step is deferred to Phase 4.

## Events

Publishers:

- `call.initiated` — `Service.RequestCall`
- `call.ringing` — inbound webhook `event=connect`, status webhook
  `RINGING`
- `call.answered` — status webhook `ACCEPTED`
- `call.ended` — terminate webhook `status=COMPLETED`,
  transcription-available webhook
- `call.failed` — terminate webhook `status=FAILED`, status webhook
  `REJECTED`
- `call.recording_created` — recording-available webhook

Consumers today:

- WebSocket bridge — pushes call events to the operator UI for live
  updates.
- Audit — every state-changing event lands a row in `audit_logs`.

## Retention

Call rows are retained indefinitely. Recording bytes stored in the
attachment store are subject to the Phase 4 media retention policy.
