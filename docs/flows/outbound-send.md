# Outbound send flow

Agent (or automation) posts a message → fullWA persists a `queued` row →
the send worker drains the queue → the WhatsApp adapter POSTs to Meta →
status webhooks close the loop.

```mermaid
sequenceDiagram
    autonumber
    participant Agent as REST client (agent UI / API)
    participant API as internal/api/rest/v1/messages
    participant App as internal/application/message.SendService
    participant DB as MySQL
    participant Queue as Kafka lane q:message.send
    participant Worker as internal/workers.SendWorker
    participant Reg as ProviderRegistry
    participant WA as whatsapp.Provider
    participant Meta as Meta Cloud API
    participant Bus as in-proc event bus

    Agent->>API: POST /api/v1/messages<br/>{conversation_id, type, ...}
    API->>App: RequestSend(SendRequest)
    App->>App: validate type + payload<br/>resolve conversation → session → endpoint → integration
    App->>DB: INSERT messages(status=queued, direction=outbound)
    App->>Queue: Enqueue{Lane:"message.send", Payload:SendJobPayload}
    App->>Bus: publish MessageSendRequested
    App-->>API: {message_id, status:"queued"}
    API-->>Agent: 202 Accepted

    Worker->>Queue: Consume("message.send", group)
    Worker->>App: ProcessSend(SendJobPayload)
    App->>App: Integrations.GetWithSecrets(org, integration_id)
    App->>Reg: Channel(provider_key, secrets)
    Reg-->>App: channel.Provider adapter
    App->>WA: SendMessage(ctx, channel.SendRequest{IdempotencyKey=msg_id})
    WA->>Meta: POST /<phone_number_id>/messages
    alt success
        Meta-->>WA: {messages:[{id: wamid...}]}
        WA-->>App: SendResult{ProviderMessageID:wamid}
        App->>DB: MessageRepo.UpdateStatus(SENT, now)
        App->>Bus: publish MessageSent
        App-->>Worker: nil (ack)
    else rate-limited / transport 5xx
        WA-->>App: *APIError (Retryable=true)
        App-->>Worker: return err (nack; queue retries with backoff)
    else permanent / auth
        WA-->>App: *APIError (Retryable=false)
        App->>DB: MessageRepo.UpdateStatus(FAILED, now)
        App->>Bus: publish MessageFailed
        App-->>Worker: nil (ack; no retry)
    end

    Meta-->>Bus: (later) delivered / read / failed status webhooks
    Bus-->>DB: UPDATE messages SET status, timestamps
    Bus-->>Bus: broadcast to WS inbox subscribers
```

## Guarantees

- Message row is inserted (with `status=queued`) **before** the queue push.
  A crash between the DB commit and the queue push leaves a durable row a
  sweeper on `(org_id, status=queued, created_at)` re-enqueues.
- No DB transaction is held across the Meta call — the row commit happens
  in `RequestSend`; the provider call happens later in `ProcessSend`.
- Enqueue is the **only** trigger for the provider call. The REST path
  never talks to Meta directly.
- `idempotency_key` from the API is echoed to the provider as
  `biz_opaque_callback_data`. When the caller omits it, the message ID is
  used — subsequent status webhooks carry this back, allowing the worker to
  reconcile without a client round-trip.
- Rate-limit and transient failures return an error from `ProcessSend`, so
  the queue redelivers with backoff. Auth / validation failures mark the
  message `failed` and return nil so the queue moves on.
- The application layer never imports a provider package — the
  `ProviderRegistry` port hands back the correct `channel.Provider` adapter
  bound to the integration's decrypted secrets.

## Endpoints

- `POST /api/v1/messages` (auth + CSRF) — enqueues a send. See the OpenAPI
  spec at `internal/api/openapi/openapi.yaml` for the request body.
- `GET /api/v1/conversations/{id}/messages` (auth) — returns paginated
  messages for a conversation, newest first.
- `GET /api/v1/conversations` (auth) — placeholder empty list until the
  full listing lands under Phase 1 Task 4.

## Queue lane

`message.send` — one job per outbound message. Payload:

```json
{
  "message_id": "01H...",
  "org_id": "01H...",
  "integration_id": "01H...",
  "provider_key": "whatsapp",
  "recipient": "+15551234567",
  "type": "text",
  "payload": {"body": "Hi"},
  "idempotency_key": "...",
  "correlation_id": "...",
  "request_id": "..."
}
```

## State transitions

`RequestSend` → `queued`.
`ProcessSend` success → `sent` (via `Message.Transition(StatusSent, now)`).
`ProcessSend` permanent failure → `failed`.
Later: provider status webhook → `delivered` → `read`, or a late `failed`.
