# Inbound message flow

End-to-end: Meta HTTP webhook → canonical `MessageReceived` → agent inbox.

```mermaid
sequenceDiagram
    autonumber
    participant Meta as Meta Cloud API
    participant Ingress as webhook HTTP handler
    participant WA as internal/providers/whatsapp
    participant HBase as HBase (raw_events)
    participant Queue as Redis Stream q:webhook.process
    participant Worker as workers/webhook
    participant App as internal/application/message
    participant DB as MySQL
    participant Bus as event bus (Envelope)
    participant WS as WebSocket hub
    participant Auto as automation engine
    participant Ana as analytics

    Meta->>Ingress: POST /webhooks/whatsapp<br/>X-Hub-Signature-256, JSON body
    Ingress->>WA: VerifySignature(headers, body, appSecret)
    alt bad signature
        WA-->>Ingress: ErrSignatureMismatch
        Ingress-->>Meta: 401
    else valid
        Ingress->>HBase: put raw body (row key: org|hh|integration|eventID)
        Ingress->>DB: INSERT INTO webhook_events (idempotent on integration_id+external_event_id)
        Ingress->>Queue: XADD q:webhook.process
        Ingress-->>Meta: 200 OK
    end

    Worker->>Queue: XREADGROUP
    Worker->>WA: ParseWebhook(body, resolver)
    WA->>WA: canonicalize each message
    WA-->>Worker: []events.Envelope
    loop per envelope
        Worker->>App: HandleInbound(envelope)
        App->>DB: FindOrCreate Contact by (org, provider, wa_id)
        App->>DB: FindOrCreate Identity
        App->>DB: FindOrCreateActive Session (org, endpoint, contact)
        App->>DB: FindOrCreateOpen Conversation (session)
        App->>DB: INSERT message metadata (unique on org+provider+provider_message_id)
        App->>HBase: put message payload (row key: org|conversation|revTs|msgID)
        App->>Bus: Publish MessageReceived envelope
    end
    Bus-->>WS: broadcast to org room + conversation subscribers
    Bus-->>Auto: match rules / trigger actions
    Bus-->>Ana: emit analytics event
```

Notes:

- Signature verification runs on the **raw** body — no re-serialisation, or the MAC breaks.
- The 200 is returned **before** parsing/queuing side-effects propagate. Idempotency (unique on `webhook_events(integration_id, external_event_id)` + unique on `messages(org, provider, provider_message_id)`) ensures replays are safe.
- The application layer never imports `internal/providers/whatsapp`. It works with `events.Envelope` / `MessageReceivedPayload` values only.
