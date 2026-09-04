# Analytics

The Analytics page has two tabs that answer two different questions:

- **Nudgeway** — how does the system Nudgeway persists look? Fed by local rollups off `messages`, `conversations`, and `calls`. Zero Meta round-trips on the read path.
- **Meta Analytics** — how does the WABA look on Meta's side? Fed by pass-through calls to the WhatsApp Business Cloud analytics APIs, scoped per integration.

The tabs are independent — one can be healthy while the other returns errors. Both surfaces gate on the `analytics.read` permission.

## Which tab do I want?

| I want to see... | Use tab |
|---|---|
| Messages my server sent today (regardless of Meta acceptance) | Nudgeway |
| Average customer response time in the last 14 days | Nudgeway |
| Sparkline of call volume for the last week | Nudgeway |
| Meta's authoritative delivery count per country | Meta Analytics → Messaging |
| Cost per conversation split by category | Meta Analytics → Pricing |
| Business-initiated vs user-initiated call counts | Meta Analytics → Calls |

## Update cadence

- **Nudgeway tab**: rollup worker writes `analytics_*_daily` tables every 15 minutes. See [Nudgeway tab](/#/analytics/nudgeway-tab).
- **Meta Analytics tab**: live pass-through — every panel is one Meta HTTP call. Latency depends on Meta.

## Related

- [Nudgeway tab (local KPIs)](/#/analytics/nudgeway-tab)
- [Meta Analytics tab (WABA proxy)](/#/analytics/meta-analytics-tab)
- [Analytics troubleshooting](/#/analytics/troubleshooting)
- Source of truth: `docs/flows/analytics-rollup.md`, `docs/domain/analytics.md`.
