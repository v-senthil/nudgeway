# Flow: Outbound WhatsApp call

An outbound call is a business-initiated call placed via the Nudgeway
operator UI. The full lifecycle spans a synchronous REST request that
persists a queued row + kicks off the Meta initiate, followed by an
asynchronous stream of webhooks that advance the row through the state
machine.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant UI as Operator UI
    participant REST as REST v1<br/>POST /api/v1/calls
    participant Svc as application/call<br/>Service
    participant Repo as MySQL calls
    participant WA as whatsapp<br/>Provider adapter
    participant Meta as Meta Cloud API
    participant WS as WebSocket bridge

    UI->>REST: POST /api/v1/calls {integration_id, to}
    REST->>Svc: RequestCall(req)
    Svc->>Svc: Resolve integration + secrets
    Svc->>Svc: Resolve calling.Provider from Registry
    Svc->>Repo: Create(row{status=queued, provider_call_id=pending:<ulid>})
    Repo-->>Svc: ok
    Svc->>WA: Provider.InitiateCall(CallRequest)
    WA->>Meta: POST /<phone_number_id>/calls (action=connect, SDP offer)
    Meta-->>WA: {call_id: wacid.*}
    WA-->>Svc: CallResult{ProviderCallID}
    Svc->>Repo: UpsertByProviderID(row{status=ringing, started_at, provider_call_id=wacid.*})
    Svc->>WS: Publish(events.CallInitiated)
    Svc-->>REST: {call_id, status=ringing}
    REST-->>UI: 202 Accepted

    Note over Meta,UI: Meta streams status webhooks…

    Meta->>REST: POST /webhooks/whatsapp<br/>statuses[]=RINGING
    REST->>Svc: ProcessInboundEvent(CallRinging)
    Svc->>Repo: UpsertByProviderID (idempotent no-op)
    Svc->>WS: Publish(events.CallRinging)

    Meta->>REST: POST /webhooks/whatsapp<br/>statuses[]=ACCEPTED
    REST->>Svc: ProcessInboundEvent(CallAnswered)
    Svc->>Repo: UpsertByProviderID(status=answered, answered_at)
    Svc->>WS: Publish(events.CallAnswered)

    Note over Meta,UI: Business hangs up → REST End call…

    UI->>REST: POST /api/v1/calls/{id}/end
    REST->>Svc: End(orgID, id)
    Svc->>WA: Provider.EndCall(providerCallID)
    WA->>Meta: POST /<phone_number_id>/calls action=terminate
    Meta-->>WA: {success:true}
    Svc->>Repo: UpdateStatus(status=completed, ended_at)

    Meta->>REST: POST /webhooks/whatsapp<br/>event=terminate status=COMPLETED
    REST->>Svc: ProcessInboundEvent(CallEnded)
    Svc->>Repo: UpsertByProviderID(duration_seconds)

    Meta->>REST: POST /webhooks/whatsapp<br/>event=call_recording_available
    REST->>Svc: ProcessInboundEvent(CallRecordingCreated)
    Svc->>Repo: AttachRecording(url, duration)
```

## Why Create-then-Upsert?

We insert the row before calling Meta so a crash between "Meta accepted"
and "we recorded the id" leaves a queued row that a sweeper can retry
(or manually reconcile). The initial insert uses a `pending:<ulid>`
placeholder for `provider_call_id` to satisfy the unique index; the
subsequent `UpsertByProviderID` overwrites it with the real wacid.

## Failure modes

- Provider returns 4xx → row marked `failed`; 502 surfaced to caller.
- Provider returns 5xx / transient → row stays queued; caller gets 502
  and retries (idempotency key ensures no duplicate placement).
- Webhook delivery delayed → row status may lag reality but eventually
  converges. The state machine is monotonic — a stale RINGING webhook
  cannot revert a completed row.

## Code references

- Handler: [`internal/api/rest/v1/calls.go`](../../internal/api/rest/v1/calls.go)
- Service: [`internal/application/call/service.go`](../../internal/application/call/service.go)
- Adapter: [`internal/providers/whatsapp/calling.go`](../../internal/providers/whatsapp/calling.go)
