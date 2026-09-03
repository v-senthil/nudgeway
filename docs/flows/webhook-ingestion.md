# Webhook ingestion — generic ingress + provider dispatch

fullWA exposes a single generic webhook ingress under `internal/webhook/`. It routes based on integration/provider derived from the URL path — `/webhooks/{provider}/{integration_id}` — and hands the raw body to the matching provider adapter.

```mermaid
sequenceDiagram
    autonumber
    participant Ext as External provider (Meta / Zoho / Twilio / ...)
    participant Ingress as internal/webhook (HTTP handler)
    participant Reg as internal/providers registry
    participant Adapter as provider.<name>
    participant HBase as HBase (raw_events)
    participant DB as MySQL (webhook_events)
    participant Queue as Redis Stream q:webhook.process
    participant Worker as workers/webhook
    participant App as internal/application/*
    participant Bus as event bus

    Ext->>Ingress: POST /webhooks/{provider}/{integration_id}
    Ingress->>Reg: Lookup(kind=channel, key={provider})
    Ingress->>Adapter: VerifySignature(headers, rawBody, secret)
    alt bad signature
        Adapter-->>Ingress: error
        Ingress-->>Ext: 401
    else valid
        Ingress->>HBase: put raw body (row key: org|hh|integration|eventID)
        Ingress->>DB: INSERT INTO webhook_events (unique on integration_id+external_event_id)
        alt duplicate
            DB-->>Ingress: duplicate key
            Ingress-->>Ext: 200 OK (idempotent replay)
        else new
            Ingress->>Queue: XADD q:webhook.process
            Ingress-->>Ext: 200 OK
        end
    end

    Worker->>Queue: XREADGROUP
    Worker->>Adapter: ParseWebhook(ctx, headers, body)
    Adapter-->>Worker: []events.Envelope
    loop per envelope
        Worker->>App: HandleEvent(envelope)
        App->>Bus: republish canonical event
    end
```

Key properties:

- The HTTP handler never types a Meta / Twilio / Zoho struct — those live inside the adapter package. The handler only knows the port surface: `VerifySignature`, `ParseWebhook`.
- Idempotency is enforced at two layers:
  1. `webhook_events(integration_id, external_event_id)` unique index — dedupes at ingress.
  2. `messages(org, provider, provider_message_id)` unique index — dedupes at persistence, so a partial replay after a crash never double-inserts a message.
- Raw envelopes are written to HBase before ACK so debugging + replay never require re-fetching from the provider.
- 200 is returned **before** application processing so providers do not retry when the queue is behind.
