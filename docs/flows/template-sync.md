# Flow: Template sync

Reconcile the local `templates` mirror against a single Integration's
provider-side template list. Triggered manually today via
`POST /api/v1/templates/sync`; a scheduled worker lands in Phase 3.

## Actors

- **REST client** — the operator's browser calling `POST /api/v1/templates/sync`.
- **`apptmpl.Service.Sync`** — application-layer entry point.
- **`ports/repository.TemplateRepo`** — local persistence.
- **`apptmpl.TemplateProvider`** — the port implementation bound to the
  target integration (WhatsApp adapter today).
- **`whatsapp.Provider.ListTemplates`** — the actual Meta Graph API call.

## Sequence

```
REST client                Service.Sync           TemplateProvider     Meta Graph API   TemplateRepo
    │                          │                        │                  │                │
    │ POST /templates/sync ───►│                        │                  │                │
    │  {integration_id}        │                        │                  │                │
    │                          │─ Integrations.Get ────────────────────────────────►│       │
    │                          │◄────────── row + secrets ──────────────────────────┤       │
    │                          │                        │                  │                │
    │                          │─ Providers.Template ──►│                  │                │
    │                          │◄──── bound provider ───┤                  │                │
    │                          │                        │                  │                │
    │                          │─ ListTemplates ───────►│                  │                │
    │                          │                        │─ GET /message_templates ─►│      │
    │                          │                        │◄─── {"data": [...]} ──────┤      │
    │                          │◄──── []Summary ────────┤                  │                │
    │                          │                        │                  │                │
    │                          │  for each summary:                                          │
    │                          │─ Upsert(canonical row) ────────────────────────────────────►│
    │                          │                                                            │
    │                          │  log {fetched, upserted}                                    │
    │◄─ 200 SyncTemplatesResponse                                                            │
```

## Idempotency

`Upsert` keys on `(org_id, integration_id, name, language)`. Running the
sync twice in a row is safe: the second call finds every row and updates
`status`, `category`, `components`, `variables`, `last_synced_at`
without changing the primary key. Local `DRAFT` rows the provider does
not know about are left alone — they haven't been submitted yet.

## Retry semantics

- **Provider transient errors** (429, 5xx) — the WhatsApp adapter's
  `client.doJSON` already retries with jittered exponential backoff (see
  [`internal/providers/whatsapp/client.go`](../../internal/providers/whatsapp/client.go)).
  A caller-visible failure means the retry budget was exhausted.
- **Persistence errors on individual rows** — logged at WARN and
  skipped; the run continues so a single malformed row does not sink the
  whole sync.
- **Auth failures** — surfaced to the REST edge as `502 provider_error`.
  The operator refreshes the access token via the Integration settings
  page and retries.

## Observability

- `logger.Info("template sync complete", ...)` emits `org_id`,
  `integration_id`, `fetched`, `upserted`.
- Every provider call also emits an execution-log event via the tracer
  (see [`internal/providers/whatsapp/tracer.go`](../../internal/providers/whatsapp/tracer.go)) — Meta API log page shows the `list_templates`
  call with latency + status code + trace id.

## Related flows

- [`outbound-send.md`](outbound-send.md) — templated outbound messages
  will look up the local Template row before assembling the Meta payload
  (Phase 3 wire-up).
- [`provider-call-recording.md`](provider-call-recording.md) — the
  tracer path every provider call goes through.
