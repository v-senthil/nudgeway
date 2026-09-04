# Call settings

Provider-agnostic per-integration call settings — WhatsApp call feature status, whether the call icon shows in the customer's chat header, weekly call hours, and callback-permission behaviour.

## Fields

| Field | Notes |
|---|---|
| `status` | WhatsApp call feature status (e.g. `ENABLED`, `DISABLED_BY_USER`, `NOT_APPLICABLE`). |
| `call_icon_visibility` | Whether the call icon is shown in the customer's chat header. |
| `call_hours.status` | On / off toggle for the weekly hours schedule. |
| `call_hours.timezone_id` | IANA tz id (e.g. `Asia/Kolkata`). |
| `call_hours.weekly_operating_hours[]` | Array of `{day_of_week, open_time, close_time}`. Times are Meta's `HHMM` format (`0900` = 09:00). |
| `callback_permission_status` | Whether the business is allowed to request callbacks. |

## How to use

1. Settings → Integrations → the row → **Call settings** drawer.
2. Toggle call feature status, call-icon visibility, and callback permission.
3. Add / remove `WeeklyHours` rows for each day the business accepts calls.
4. Save.

## API

### Read

```
GET /api/v1/integrations/{id}/call-settings
```

Response `200`:

```json
{
  "status": "ENABLED",
  "call_icon_visibility": "DEFAULT",
  "call_hours": {
    "status": "ENABLED_WITH_CUSTOM_HOURS",
    "timezone_id": "Asia/Kolkata",
    "weekly_operating_hours": [
      {"day_of_week": "MONDAY", "open_time": "0900", "close_time": "1800"},
      {"day_of_week": "TUESDAY", "open_time": "0900", "close_time": "1800"}
    ]
  },
  "callback_permission_status": "ENABLED"
}
```

Requires `integrations.manage`.

### Write

```
PUT /api/v1/integrations/{id}/call-settings
Content-Type: application/json

{
  "status": "ENABLED",
  "call_icon_visibility": "DEFAULT",
  "call_hours": {
    "status": "ENABLED_WITH_CUSTOM_HOURS",
    "timezone_id": "Asia/Kolkata",
    "weekly_operating_hours": [
      {"day_of_week": "MONDAY", "open_time": "0900", "close_time": "1800"}
    ]
  },
  "callback_permission_status": "ENABLED"
}
```

Response `200` returns the reconciled settings. Requires CSRF + `integrations.manage`.

## Per-recipient call permission

Separately, `GET /api/v1/integrations/{id}/call-permission?to=<E.164>` (operationId `getIntegrationCallPermission`) returns the recipient's current WhatsApp user-call-permission chip (`no_permission | temporary | permanent` + `expiration_time` for temporary). Used by the "New call" affordance to gate the Call button. Gated on `calls.read` rather than `integrations.manage`.

## MCP

| operationId | Purpose |
|---|---|
| `getIntegrationCallSettings` | Read call feature status + hours + callback state. |
| `updateIntegrationCallSettings` | Write and return the reconciled state. |
| `getIntegrationCallPermission` | Look up one recipient's call-permission chip. |

## Troubleshooting

- **`status` won't flip to `ENABLED`** — Meta gates the WhatsApp calling feature per-WABA. Contact your Meta rep.
- **Hours look off by an hour** — verify `timezone_id`; Meta interprets `open_time` / `close_time` as local in that zone, not UTC.
- **`501 not wired`** on `getIntegrationCallPermission` — expected when the adapter doesn't wire the call-permission lookup for this deployment (e.g. non-WhatsApp channel).

## Related

- [Integrations overview](/#/integrations/overview)
- [Calls overview](/#/calls/overview)
- [Call permissions](/#/calls/call-permissions)
