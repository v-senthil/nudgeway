# Outbound send flow

Agent (or automation) posts a message → Meta accepts → status webhooks close the loop.

```mermaid
sequenceDiagram
    autonumber
    participant Agent as REST client (agent UI / API)
    participant API as internal/api/rest/v1/messages
    participant App as internal/application/message
    participant DB as MySQL
    participant Queue as Redis Stream q:message.send
    participant Worker as workers/messagesend
    participant Reg as internal/providers registry
    participant WA as whatsapp.Provider
    participant Meta as Meta Cloud API
    participant Bus as event bus
    participant WS as WebSocket hub

    Agent->>API: POST /v1/conversations/{id}/messages<br/>Idempotency-Key
    API->>App: SendMessage(canonical req)
    App->>App: validate + normalize
    App->>DB: INSERT messages (status=queued, provider_message_id=NULL)
    App->>Bus: MessageCreated
    App->>Queue: XADD q:message.send (correlation + causation)
    App-->>API: 202 { message_id, status: "queued" }
    API-->>Agent: 202 Accepted

    Worker->>Queue: XREADGROUP
    Worker->>Reg: Lookup(kind=channel, key=provider)
    Worker->>WA: SendMessage(ctx, SendRequest)
    WA->>Meta: POST /<phone_number_id>/messages
    alt success
        Meta-->>WA: {messages:[{id: wamid...}]}
        WA-->>Worker: SendResult{ProviderMessageID}
        Worker->>DB: UPDATE messages SET status=sent, sent_at=NOW, provider_message_id=wamid
        Worker->>Bus: MessageSent
    else transient/rate-limited
        WA-->>Worker: *APIError (Retryable)
        Worker->>Queue: XADD (retry with backoff)
    else permanent/auth
        WA-->>Worker: *APIError (permanent)
        Worker->>DB: UPDATE messages SET status=failed
        Worker->>Bus: MessageFailed
    end

    Meta-->>Bus: (later) delivered / read / failed webhooks
    Bus-->>DB: UPDATE messages SET status, timestamps
    Bus-->>WS: broadcast status update to conversation subscribers
```

Guarantees:

- Message row is inserted (with `status=queued`) **before** the queue push — a crash between DB and queue is recovered by a sweeper on the `(org_id, status)` index.
- No DB transaction is held across the Meta call.
- `Idempotency-Key` from the API is echoed as `biz_opaque_callback_data` on the Meta payload — subsequent status webhooks carry it back, allowing the worker to reconcile without a client roundtrip.
