# Webhook ingestion — generic ingress + provider dispatch

fullWA exposes a single generic webhook ingress at `internal/webhook/` mounted by the REST router at:

- `GET  /webhooks/{provider}/{integration_id}` — Meta-style subscription verification handshake.
- `POST /webhooks/{provider}/{integration_id}` — signed delivery.

Both endpoints are **unauthenticated at the HTTP layer** (external providers cannot present our session cookie or CSRF token). Authenticity is enforced per-provider through a `webhook.SignatureVerifier` interface implemented by each adapter — for WhatsApp, that is `whatsapp.VerifySignature` (HMAC-SHA256 over the raw body against Meta's `X-Hub-Signature-256` header).

```mermaid
sequenceDiagram
    autonumber
    participant Ext as External provider (Meta / Twilio / …)
    participant Ingress as internal/webhook.Ingress
    participant Repo as IntegrationRepo (mysql)
    participant Adapter as provider.<name> SignatureVerifier
    participant DB as MySQL webhook_events
    participant Queue as Kafka fullwa.jobs.webhook.process
    participant Worker as workers/webhook
    participant App as internal/application/*
    participant Bus as event bus

    Ext->>Ingress: POST /webhooks/{provider}/{integration_id} (raw JSON, ≤1 MiB)
    Ingress->>Ingress: MaxBytesReader(1 MiB) → 413 on overflow
    Ingress->>Repo: GetWithSecrets(integration_id) → Integration + {access_token, app_secret, verify_token}
    Note over Ingress,Repo: org_id derived from the row, never from the URL
    Ingress->>Adapter: VerifySignature(headers, rawBody, app_secret)
    alt bad signature / no verifier
        Adapter-->>Ingress: error
        Ingress-->>Ext: 401 problem+json
    else valid
        Ingress->>Ingress: external_event_id = sha256(raw_body)
        Ingress->>DB: INSERT webhook_events (UNIQUE integration_id+external_event_id)
        alt duplicate
            DB-->>Ingress: created=false
            Ingress-->>Ext: 200 (idempotent replay)
        else new
            Ingress->>Queue: Enqueue(lane=webhook.process, payload={provider, integration_id, org_id, event_id, headers, raw_body})
            Ingress-->>Ext: 200 (empty body)
        end
    end

    Ext->>Ingress: GET /webhooks/{provider}/{integration_id}?hub.mode=subscribe&hub.verify_token=…&hub.challenge=…
    Ingress->>Repo: GetWithSecrets(integration_id) → {verify_token}
    alt token matches
        Ingress-->>Ext: 200 text/plain echoing hub.challenge
    else mismatch
        Ingress-->>Ext: 403 problem+json
    end

    Worker->>Queue: Consume(lane=webhook.process)
    Worker->>Adapter: ParseWebhook(ctx, headers, body)
    Adapter-->>Worker: []events.Envelope
    loop per envelope
        Worker->>App: HandleEvent(envelope)
        App->>Bus: republish canonical event
    end
    Worker->>DB: MarkProcessed(webhook_event_id)
```

Key properties (as shipped in Phase 1 Task 2):

- The HTTP handler never types a Meta / Twilio / Zoho struct — those live inside the adapter package. The handler only knows the port surface: `SignatureVerifier.VerifySignature` at ingress, `channel.Provider.ParseWebhook` at the worker.
- **Signature verification runs before any parsing.** A forged body cannot reach the parser.
- **Body cap: 1 MiB** via `http.MaxBytesReader`. Larger requests get 413 problem+json.
- Idempotency is enforced at two layers:
  1. `webhook_events(integration_id, external_event_id)` UNIQUE index — dedupes at ingress. `external_event_id = sha256(raw_body)` because Meta envelopes carry no top-level id.
  2. `messages(org, provider, provider_message_id)` UNIQUE index — dedupes at persistence, so a partial replay after a crash never double-inserts a message.
- Raw envelopes are written to `webhook_events.raw_body` **before** the 200 ACK so debugging + replay never require re-fetching from the provider.
- 200 is returned **before** application processing so providers do not retry when the queue is behind. If enqueue fails after the row is persisted, the ingress still ACKs 200 — a reconciler over `Status=received` rows picks it up.
- Every log line carries `request_id`, `provider`, `integration_id`, `org_id`, `event_id`, `sig_ok`, `dup`.
- `org_id` is derived from the Integration row, **never** trusted from the URL. Posting a WhatsApp-signed body to a Twilio integration id gets 404.
