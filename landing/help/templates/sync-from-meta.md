# Sync status from Meta

Templates are mirrored from Meta. Meta pushes state transitions via webhook, but pushes can be missed (throttled, dropped, disabled webhook). `syncTemplates` reconciles the local mirror by pulling every template from Meta and upserting into the `templates` table.

## When to use

- A `PENDING` template has been stuck for more than a few minutes and you want to check if Meta already moved it.
- After enabling a fresh integration — pulls the org's existing templates that were created outside Nudgeway.
- After a suspected webhook gap (webhooks disabled, tunnel down, etc.).
- Periodic reconciliation from a cron/scheduled job.

## When status flips

- `PENDING → APPROVED` — usable in the composer.
- `PENDING → REJECTED` — Meta returned a reason; check the `reason` field in the template body.
- `APPROVED → PAUSED` — quality gate; template still exists but Meta throttles sends.
- `APPROVED → DISABLED` — quality gate fully tripped; sends will fail.

Local `DRAFT` rows the provider does not know about are left alone — you won't lose in-progress drafts.

## API

**operationId**: `syncTemplates`

```
POST /api/v1/templates/sync
```

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/templates/sync' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{ "integration_id": "01M1MC4KFJQ33YQWKPT7HKZNYC" }'
```

Response (`SyncTemplatesResponse`):

```json
{ "fetched": 24, "upserted": 24 }
```

`fetched` — templates Meta returned. `upserted` — rows we inserted or updated locally.

## How to use (UI)

Templates → **Sync from Meta**. A spinner runs while the reconciliation is in flight; the list refreshes with the new statuses on completion. Toast confirms `fetched` and `upserted` counts.

## MCP

Call the `syncTemplates` tool with `{ "body": { "integration_id": "<integration-ULID>" } }`.

## Troubleshooting

- **`400 missing integration_id`** — the body is required, and `integration_id` is the only field.
- **`0 fetched`** — Meta returned no templates. Either the WABA has none, or the access token doesn't have `whatsapp_business_management`.
- **`502 provider error`** — Meta returned a transient error. Retry after a short back-off; check the [Provider calls log](/#/audit-telemetry/provider-calls) for the exact response.
- **Sync succeeds but statuses look stale** — the response is the count, not the diff. Re-list via `getTemplates` to see the current state.

## Related

- [Templates overview](/#/templates/overview) — lifecycle.
- [Submit for Meta review](/#/templates/submit-for-review) — the flow that creates a `PENDING`.
- [Troubleshooting](/#/templates/troubleshooting) — rejection + status issues.
