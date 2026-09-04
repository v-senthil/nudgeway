# Official Business Account (OBA)

Meta's Official Business Account badge — the green checkmark next to the business name in WhatsApp — unlocks several capabilities Nudgeway depends on, notably the [Groups](/#/groups/overview) API. This page tracks the OBA application lifecycle for one integration.

## Status enum

| Value | Meaning |
|---|---|
| `NOT_APPLIED` | No application on file. Click **Apply** to file one. |
| `PENDING` | Application filed, awaiting Meta review (typically 1–5 business days). |
| `APPROVED` | Badge granted. OBA-gated APIs are usable. |
| `REJECTED` | Meta declined. `status_message` carries the reason. Re-apply after resolving. |
| `CANCELLED` | Application withdrawn (via **Withdraw** or by Meta). |

## How to use

1. Settings → Integrations → the row → **OBA** section.
2. If `NOT_APPLIED` — click **Apply**. Nudgeway files the application with Meta.
3. Wait. The status polls on drawer open.
4. If `PENDING` and you want to abort — click **Withdraw**.
5. Once `APPROVED`, sync your [Groups](/#/groups/list-sync).

## API

### Read status

```
GET /api/v1/integrations/{id}/oba-status
```

Response `200`:

```json
{ "oba_status": "PENDING", "status_message": "" }
```

### Apply

```
POST /api/v1/integrations/{id}/oba-status/apply
```

No body. Response `200` reflects the resulting status (usually `PENDING`).

### Withdraw

```
POST /api/v1/integrations/{id}/oba-status/withdraw
```

No body. Response `200` reflects the resulting status (usually `CANCELLED`).

All three require `integrations.manage` (Apply / Withdraw also require CSRF).

## MCP

| operationId | Purpose |
|---|---|
| `getIntegrationOBAStatus` | Read the current application state. |
| `applyIntegrationOBA` | File an OBA application. |
| `withdrawIntegrationOBA` | Cancel an in-flight application. |

## Troubleshooting

- **`REJECTED` with a generic reason** — Meta doesn't always give actionable detail. Common causes: business verification incomplete, brand-name conflict, missing website. Fix and re-apply after 30 days.
- **`APPROVED` but Groups still 502** — Meta occasionally lags on propagating OBA to the Cloud API. Wait 24 hours and re-run [group sync](/#/groups/list-sync).
- **Apply returns `502`** — the WABA is in review or under Meta enforcement. Check [Meta API execution log](/#/audit-telemetry/provider-calls) filtered on `operation=apply_oba`.

## Related

- [Integrations overview](/#/integrations/overview)
- [Groups overview](/#/groups/overview)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
