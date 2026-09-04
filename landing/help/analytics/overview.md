# Analytics

The Analytics page has two tabs that answer two different questions:

- **Nudgeway** — how does traffic look on your Nudgeway workspace? Numbers here come from your own send / receive activity and refresh every 15 minutes.
- **Meta Analytics** — how does the WhatsApp Business Account look on Meta's side? These panels are live views onto Meta's own analytics APIs, scoped to one integration at a time.

The two tabs are independent. If Meta's side is slow, the Nudgeway tab still works, and vice versa. Both tabs are gated on the "analytics.read" permission — an org admin can grant this if you don't see them.

## Which tab do I want?

| I want to see... | Use tab |
|---|---|
| Messages my workspace sent today, regardless of Meta acceptance | Nudgeway |
| Average customer response time over the last 14 days | Nudgeway |
| A weekly sparkline of call volume | Nudgeway |
| Meta's authoritative delivery count per country | Meta Analytics -> Messaging |
| Cost per conversation split by category | Meta Analytics -> Pricing |
| Business-initiated vs user-initiated call counts | Meta Analytics -> Calls |

## How fresh is the data?

- **Nudgeway tab**: refreshed every 15 minutes by a background job. If you just sent your first message, wait one tick before the KPIs light up.
- **Meta Analytics tab**: live — each panel calls Meta at the moment you load it. Latency depends on Meta.

## Related

- [Nudgeway tab (local KPIs)](#/analytics/nudgeway-tab)
- [Meta Analytics tab (WABA proxy)](#/analytics/meta-analytics-tab)
- [Analytics troubleshooting](#/analytics/troubleshooting)
