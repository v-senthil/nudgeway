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
            opt media envelope (image/video/audio/document/sticker)
                App->>WA: Downloader.Download(provider, integration, media_id)
                WA-->>App: io.ReadCloser + content-type
                App->>App: attachments.Store.Put → sha256 key
                note over App: metadata.attachment_key / content_type / file_size stamped
            end
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

## Media persistence

Meta only carries a `media_id` handle in the webhook — the raw bytes
live behind a short-lived signed URL that must be fetched with the
integration's access token before it expires. The `InboundService`
handles this via the `AttachmentDownloader` port so no provider package
is imported at the application layer:

1. On each `MessageReceived` whose `Payload` is a `MediaPayload` with a
   non-empty `MediaID`, the service calls
   `Downloader.Download(ctx, providerKey, integrationID, mediaID)`.
2. The returned `io.ReadCloser` is streamed into
   `attachments.Store.Put(ctx, contentType, r)`. The dev implementation
   (`internal/infrastructure/attachments.LocalFS`) is content-addressed
   by SHA-256, sharded 2/2 under `Config.Root`, with a `.contenttype`
   sidecar file recording the MIME string.
3. The resulting key + content-type + size are stamped on
   `msg.metadata` as `attachment_key`, `content_type`, `file_size`.
   The REST `MessageDTO.MediaURL` becomes `/api/v1/media/<key>` for
   downloaded media and falls back to the provider-native URL only
   when the download was skipped or failed.
4. Download / store failures WARN-log and swallow — the message row
   still commits, and the browser renders "Attachment unavailable"
   instead of blocking the thread.

`GET /api/v1/media/{key}` (and `HEAD`) streams the bytes back with
`Cache-Control: private, max-age=86400`. The route is auth-gated so
downloaded media stays inside the tenant boundary even if the
content-addressed key leaks.

## Mark-as-read callback

Operators triggering "message read" on the business side (the blue-tick on
the customer's phone) drives:

```mermaid
sequenceDiagram
    autonumber
    participant UI as web/Thread.tsx
    participant Hook as useMarkConversationRead
    participant REST as POST /conversations/{id}/read
    participant App as application/message.ReadService
    participant DB as MySQL
    participant WA as internal/providers/whatsapp
    participant Meta as Meta Cloud API

    UI->>Hook: open conversation w/ unread inbound (throttled 5s)
    Hook->>REST: POST /api/v1/conversations/{id}/read
    REST->>App: ReadService.MarkConversationRead(org, conv, cap=50)
    App->>DB: MessageRepo.ListByConversation(org, conv, limit=50)
    loop each inbound with wamid and read_at IS NULL
        App->>DB: Conversations/Sessions/Endpoints/Integrations.Get*
        App->>WA: provider.MarkAsRead(ctx, wamid)
        WA->>Meta: POST /{phone_number_id}/messages<br/>{messaging_product, status:"read", message_id}
        Meta-->>WA: {"success": true}
        App->>DB: MessageRepo.UpdateStatus(read, now)
    end
    REST-->>UI: 204
```

Meta does not deliver a status callback back to us for a business-side
read — the read event exists only on the customer's client. `ReadService`
therefore stamps `messages.read_at` locally so the row's status is
authoritative for the inbox unread-count.
