# Call settings

Configure whether WhatsApp calls are enabled on this integration, whether the call icon appears in the customer's chat header, what your call hours are, and whether the business can request callbacks.

## Fields

| Field | Notes |
|---|---|
| **Call feature status** | Enabled, disabled, or "not applicable" if Meta hasn't yet enabled calling on this WABA. |
| **Call icon visibility** | Whether the phone-icon shows up in the customer's chat header. |
| **Call hours toggle** | Turn the weekly hours schedule on or off. |
| **Timezone** | IANA timezone name, e.g. `Asia/Kolkata`. Meta interprets your open / close times in this zone. |
| **Weekly hours** | Per-day open and close times, in 24-hour format. |
| **Callback permission** | Whether your business is allowed to request callbacks from customers. |

## How to use

1. Click **Settings** -> **Integrations** and pick the integration.
2. Open the **Call settings** drawer.
3. Toggle the call feature, call-icon visibility, and callback permission as needed.
4. If you're using scheduled hours, set the timezone and add one row per day of the week you want to accept calls. Times are 24-hour (e.g. 09:00 to 18:00).
5. Click **Save**.

## Troubleshooting

- **Call feature status won't flip to Enabled** — Meta controls this per WABA. Contact your Meta representative to have calling enabled on your business account.
- **Hours look off by an hour** — check the timezone field. Meta reads your open / close times as local to that zone, not UTC. Setting the timezone wrong shifts everything.
- **Callback permission stays disabled after saving** — Meta gates this per WABA too. If you've toggled it on and it flips back, your WABA isn't approved for it — contact your Meta representative.
- **"Not supported" banner on the drawer** — the integration isn't a WhatsApp integration (calling is WhatsApp-only in Nudgeway today).

## Related

- [Integrations overview](#/integrations/overview)
- [Calls overview](#/calls/overview)
- [Call permissions](#/calls/call-permissions)
