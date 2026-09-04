# Meta Analytics tab (WABA proxy)

The Meta Analytics tab is a live view onto Meta's own WhatsApp Business Cloud analytics. Every panel is a fresh call to Meta scoped to whichever integration you have selected. Requires the "analytics.read" permission.

Three sub-sections ship today:

- **Messaging** — sent and delivered counts.
- **Calls** — count, cost, and average duration.
- **Pricing** — cost and volume, split by category, type, and volume tier.

## How to use

1. Click **Analytics** in the top nav, then the **Meta Analytics** tab.
2. Use the **integration picker** in the top-left to choose which WhatsApp integration you want to look at. The picker pre-fills the phone number automatically.
3. Use the **quick range** buttons for 7d / 30d / 90d, or fill the date fields for a custom window.
4. Use the **granularity** drop-down to switch between day, half-hour, and month buckets. Available granularities depend on the sub-section.
5. If you need to look at a subset of your numbers, type comma-separated E.164 numbers without a leading `+` into the phone-numbers box.

Panels refresh whenever you change any of the filters.

## Where to find phone numbers, WABA ids, and template ids

You may need to reference these when a Meta error message includes an id you don't recognize:

- **Phone Number ID** — in the Meta App Dashboard, open your App -> WhatsApp -> **API Setup** -> the number under "From".
- **WABA ID** — same **API Setup** page, at the top under "WhatsApp Business Account ID".
- **Template IDs** — WhatsApp Manager -> Message Templates.

## Troubleshooting

- **All panels blank** — the integration picker may still be loading. Wait a couple of seconds and refresh. If it stays blank, try widening the range to the 90-day preset — the WABA may have no activity in the current range.
- **A panel shows a red error banner mentioning `phone_numbers is required`** — the phone number didn't auto-fill. Type your WhatsApp number in E.164 format (no `+`) into the phone-numbers box, or re-select the integration.
- **A panel shows a red banner mentioning "invalid range"** — check that the start date is before the end date, and that the range isn't sub-hourly outside the half-hour granularity.
- **Pricing tier column is empty for some rows** — that's expected for free-tier, free-entry-point, and free-customer-service pricing types. Meta doesn't report a tier for free messages.
- **You switch integrations and the panels don't refresh** — nudge the range picker or press Cmd+Shift+R (Ctrl+Shift+R on Windows) to force a full reload.
- **A panel shows an error you don't understand** — an admin can open **Settings** -> **Audit** -> [Meta API execution log](#/audit-telemetry/provider-calls) to see Meta's raw reply for the exact call.

## Related

- [Analytics overview](#/analytics/overview)
- [Nudgeway tab](#/analytics/nudgeway-tab)
- [Meta API execution log](#/audit-telemetry/provider-calls)
