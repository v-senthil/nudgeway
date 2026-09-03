# Inbound message flow

End-to-end: Meta HTTP webhook → canonical `MessageReceived` → agent inbox.

```mermaid
sequenceDiagram
    autonumber
    participant Meta as Meta Cloud API
    participant Ingress as internal/webhook (ingress)
    participant WA as internal/providers/whatsapp
    participant HBase as HBase (raw_events)
    participant Queue as Redis Stream q:webhook.process
    participant Pool as workers.Pool
    participant Worker as workers.WebhookWorker
    participant App as application/message.InboundService
    participant Reg as webhook.ProviderLookup
    participant DB as MySQL
    participant Bus as eventbus.Publisher
    participant WS as WebSocket hub
    participant Auto as automation engine
    participant Ana as analytics

    Meta->>Ingress: POST /webhooks/whatsapp/:integration_id<br/>X-Hub-Signature-256, JSON body
    Ingress->>WA: VerifySignature(headers, body, appSecret)
    alt bad signature
        WA-->>Ingress: ErrSignatureMismatch
        Ingress-->>Meta: 401
    else valid
        Ingress->>HBase: put raw body (row key: org|hh|integration|eventID)
        Ingress->>DB: INSERT INTO webhook_events<br/>(idempotent on integration_id+external_event_id)
        Ingress->>Queue: XADD q:webhook.process<br/>{provider, integration_id, event_id, raw_body}
        Ingress-->>Meta: 200 OK
    end

    Pool->>Worker: spawn N bounded goroutines
    Worker->>Queue: Consume(ctx, "webhook.process", group, handle)
    Queue-->>Worker: queue.Job {payload: WebhookJobPayload JSON}
    Worker->>App: ProcessRaw(ctx, provider, integrationID, eventID, rawBody)
    App->>DB: IntegrationRepo.GetWithSecrets → org_id + adapter secrets
    App->>Reg: ProviderLookup(provider) → channel.Provider
    App->>WA: provider.ParseWebhook(ctx, nil, rawBody)
    WA-->>App: []events.Envelope
    loop per envelope
        alt MessageReceived
            App->>DB: BusinessEndpointRepo.FindByExternalID(org, provider, phone_number_id)
            App->>DB: IdentityRepo.FindOrCreate(org, provider, normalized_phone)
            opt identity was created
                App->>DB: ContactRepo.Upsert(new contact)
            end
            App->>DB: SessionRepo.FindOrCreateActive(org, endpoint, contact)
            App->>DB: ConversationRepo.FindOrCreateOpen(org, session, contact)
            App->>DB: MessageRepo.Create (UNIQUE(org, provider_message_id))
            note over App,DB: Duplicate-key on Create → swallowed as success (idempotency)
            App->>Bus: Publish MessageReceived envelope
        else MessageSent/Delivered/Read/Failed
            App->>DB: MessageStatusByPMI.UpdateStatusByProviderMessageID(org, wamid, status, at)
            App->>Bus: Publish MessageDelivered / MessageRead / MessageFailed
        end
    end
    alt process succeeded
        App->>DB: WebhookEventRepo.MarkProcessed(event_id)
        App-->>Worker: nil
        Worker-->>Queue: ACK
    else permanent error (e.g. integration not found, endpoint not provisioned)
        App->>DB: WebhookEventRepo.MarkFailed(event_id, err)
        App-->>Worker: nil
        Worker-->>Queue: ACK (do not retry forever)
    else transient error (MySQL down, network)
        App->>DB: WebhookEventRepo.MarkFailed(event_id, err)
        App-->>Worker: err
        Worker-->>Queue: NACK → consumer redelivers with backoff
    end
    Bus-->>WS: broadcast to org room + conversation subscribers
    Bus-->>Auto: match rules / trigger actions
    Bus-->>Ana: emit analytics event
```

## Layered responsibilities

- **`internal/webhook`** (ingress) — provider-agnostic HTTP entry point.
  Verifies signatures via the adapter, persists the raw body, ACKs 200,
  enqueues a `WebhookJobPayload` on `q:webhook.process`. Also owns
  `ProviderLookup(providerKey)` — the runtime channel-provider registry
  that returns a `channel.Provider` implementation for a stable key.
- **`internal/workers`** — bounded goroutine pool + the `WebhookWorker`
  that decodes jobs and drives `InboundService.ProcessRaw`. The pool is
  the only sanctioned goroutine-spawning point in the codebase
  (`CLAUDE.md` §11).
- **`internal/application/message`** — the `InboundService`. Loads the
  integration, resolves the provider adapter via the injected lookup,
  calls `ParseWebhook`, and orchestrates domain-level persistence per
  envelope. Never imports any provider package.
- **`internal/providers/whatsapp`** — the concrete adapter. Owns
  Meta-shape parsing; produces canonical `events.Envelope` values.
- **`internal/domain/*`** — pure canonical types. No infra imports.

## Idempotency + failure semantics

- **Signature verification** runs on the **raw** body — no
  re-serialisation, or the MAC breaks.
- **Ingress idempotency**: `UNIQUE(integration_id, external_event_id)` on
  `webhook_events` absorbs Meta redeliveries at the door.
- **Message idempotency**: `UNIQUE(org, provider, provider_message_id)` on
  `messages`. The InboundService catches duplicate-key errors from
  `MessageRepo.Create` via `IsDuplicateMessage` and treats them as
  success — no double publish.
- **Permanent errors** (integration deleted, provider not registered,
  business endpoint not provisioned, malformed payload) mark the
  `webhook_events` row failed and ACK the queue job — no retry storm.
- **Transient errors** (MySQL down, network) mark the row failed with
  the last error and return the error so the consumer redelivers per
  its backoff policy. The reconciler picks up any row still in
  `status='received'` past a threshold.
- The application layer never imports `internal/providers/whatsapp` —
  it works with `events.Envelope` values and looks up providers by
  string key via `webhook.ProviderLookup`.
- Persistence is per-envelope; **no DB transaction spans the provider
  call**, honouring the prime directive in `CLAUDE.md` §2.
