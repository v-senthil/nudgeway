# Nudgeway tab (local KPIs)

The Nudgeway tab reads from the `analytics_*_daily` tables written by the rollup worker every 15 minutes. Reads are cheap — no ONLINE `SUM()` over raw `messages` / `calls` — and there are no third-party dependencies on the read path.

## KPIs

| Card | Definition |
|---|---|
| **Messages total** | Sum of `messages_daily.count` over the range, pan-provider. |
| **Delivery rate** | `delivered / sent` as an integer percentage in the 0..100 range. Zero when Sent is zero. |
| **Response time p50** | Coarse p50 of the per-day average time (seconds) between an inbound message and the next outbound reply. |
| **Conversations opened** | Sum of `conversations_daily.opened` over the range. |
| **Calls total** | Sum of `calls_daily.total` over the range. |
| **Calls answered** | Sum of `calls_daily.answered`. |
| **Avg call duration** | Weighted average over answered calls only. |

Each KPI has a matching sparkline underneath — one point per day in the range.

## How to use

1. **Analytics** in the top nav → **Nudgeway** tab (default).
2. Range picker in the top-right — the default is the last 14 days ending today.
3. Cards refresh automatically as the range changes.

## API

### Overview aggregate

```
GET /api/v1/analytics/overview?from=2026-08-22&to=2026-09-04
```

Response `200`:

```json
{
  "messages_total": 61,
  "delivery_rate_pct": 23,
  "response_time_seconds_p50": 4176,
  "conversations_opened": 2,
  "calls_total": 6,
  "calls_answered": 6,
  "calls_avg_duration_seconds": 22
}
```

### Series (one line chart)

```
GET /api/v1/analytics/series?kind=messages_daily&from=2026-08-22&to=2026-09-05
```

`kind` values: `messages_daily | delivery_rate | conversations_opened`.

Response `200`:

```json
{
  "name": "messages",
  "points": [
    {"day": "2026-09-05", "value": 12},
    {"day": "2026-09-04", "value": 7}
  ]
}
```

Both endpoints require `analytics.read`.

## Rollup timing

Every 15 minutes the runner processes yesterday + today + tomorrow (local time) for every tenant to catch tz-boundary drift. Idempotent — writes are `ON DUPLICATE KEY UPDATE`. A missed tick simply defers by 15 minutes; nothing is ever lost. See `docs/flows/analytics-rollup.md`.

For an immediate refresh in dev, restart the server: the runner fires a boot tick before the first 15-minute interval.

## Troubleshooting

- **Cards all zero** — rollup has not fired yet (up to 15 minutes after first traffic). See [Analytics troubleshooting](/#/analytics/troubleshooting).
- **Sparkline maxes at 2 on a flat line** — the chart has a floor of 1 with 15% headroom; all-zero data reads as "max 2", not real data.
- **KPI fallback** — if a day's per-direction detail rows are present but the pan-direction "all" row is missing (older rollups), both KPI and series fold detail rows for that day.

## Related

- [Analytics overview](/#/analytics/overview)
- [Meta Analytics tab](/#/analytics/meta-analytics-tab)
- [Analytics troubleshooting](/#/analytics/troubleshooting)
