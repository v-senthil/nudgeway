# Flow: Inbound WhatsApp call

An inbound call is a user-initiated call from a WhatsApp customer to the
business phone number. The full lifecycle is driven by Meta webhooks
against the `calls` field on the `whatsapp_business_account` object.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant User as WhatsApp user
    participant Meta as Meta Cloud API
    participant Ingress as fullWA<br/>webhook ingress
    participant Parser as whatsapp<br/>ParseCallWebhook
    participant Svc as application/call<br/>Service
    participant Repo as MySQL calls
    participant WS as WebSocket bridge

    User->>Meta: Dials WhatsApp Business number
    Meta->>Ingress: POST /webhooks/whatsapp<br/>changes[].field=calls, event=connect
    Ingress->>Ingress: VerifySignature(X-Hub-Signature-256)
    Ingress->>Parser: ParseCallWebhook(rawBody, resolver)
    Parser-->>Ingress: []events.Envelope{CallRinging}
    Ingress->>Svc: ProcessInboundEvent(envelope)
    Svc->>Repo: UpsertByProviderID(row{status=ringing})
    Repo-->>Svc: ok
    Svc->>WS: Publish(events.CallRinging)
    WS-->>Ingress: (async fan-out to operator UIs)

    Note over User,Meta: Operator answers via UI...

    Meta->>Ingress: POST /webhooks/whatsapp<br/>statuses[]=ACCEPTED
    Ingress->>Parser: ParseCallWebhook
    Parser-->>Ingress: []events.Envelope{CallAnswered}
    Ingress->>Svc: ProcessInboundEvent
    Svc->>Repo: UpsertByProviderID(status=answered, answered_at)
    Svc->>WS: Publish(events.CallAnswered)

    Note over User,Meta: Call ends…

    Meta->>Ingress: POST /webhooks/whatsapp<br/>event=terminate, status=COMPLETED
    Ingress->>Parser: ParseCallWebhook
    Parser-->>Ingress: []events.Envelope{CallEnded}
    Ingress->>Svc: ProcessInboundEvent
    Svc->>Repo: UpsertByProviderID(status=completed, ended_at, duration)
    Svc->>WS: Publish(events.CallEnded)

    Meta->>Ingress: POST /webhooks/whatsapp<br/>event=call_recording_available
    Ingress->>Parser: ParseCallWebhook
    Parser-->>Ingress: []events.Envelope{CallRecordingCreated}
    Ingress->>Svc: ProcessInboundEvent
    Svc->>Repo: AttachRecording(url, duration)
```

## Idempotency

`UpsertByProviderID` matches on `(org_id, provider, provider_call_id)`.
Redelivery of the same webhook advances state monotonically — a terminal
status cannot be un-set.

## Failure modes

- Unknown `phone_number_id` → parser returns an empty envelope list, the
  ingress ACKs 200. The row is dropped rather than misattributed.
- Endpoint delete-then-webhook race → row is persisted with a null
  `business_endpoint_id`.
- Signature verification failure → ingress returns 401 before touching
  the parser.

## Code references

- Parser: [`internal/providers/whatsapp/calling.go`](../../internal/providers/whatsapp/calling.go)
  (`ParseCallWebhook`)
- Service: [`internal/application/call/service.go`](../../internal/application/call/service.go)
  (`ProcessInboundEvent`)
- Repo: [`internal/infrastructure/mysql/calls.go`](../../internal/infrastructure/mysql/calls.go)
  (`UpsertByProviderID`, `AttachRecording`)
