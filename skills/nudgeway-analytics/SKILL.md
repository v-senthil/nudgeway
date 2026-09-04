---
name: nudgeway-analytics
description: KPI cards + sparklines — messages / delivery rate / response time / conversations opened / calls total / calls answered / avg call duration. Backed by daily rollups written every 15 minutes.
trigger: User asks about analytics, message volume, delivery rate, call KPIs, or the analytics dashboard.
---

# Nudgeway analytics skill

## Overview

Analytics is powered by a daily-rollup worker that summarises `messages`, `calls`, and `conversations` into `analytics_*_daily` tables every 15 minutes. Reads are cheap — no ONLINE `SUM()` over the raw tables. Rows use `day` as a DATE column keyed to server-local midnight; the REST layer takes UTC-anchored ISO dates and returns per-day points.

## Surface

Analytics endpoints aren't yet checked into openapi.yaml. Call the REST paths directly:

```
GET /api/v1/analytics/overview?from=YYYY-MM-DD&to=YYYY-MM-DD
GET /api/v1/analytics/series?kind=<kind>&from=&to=&provider=
GET /api/v1/settings/provider-calls               — per-integration Meta API log
GET /api/v1/settings/audit                        — org-wide audit log
```

Series `kind` values: `messages_daily`, `delivery_rate`, `conversations_daily`, `calls_daily`.

## Patterns

### Overview for the last 14 days

```bash
GET /api/v1/analytics/overview?from=2026-08-22&to=2026-09-04
→ {
    "messages_total": 61,
    "delivery_rate_pct": 23,
    "response_time_seconds_p50": 4176,
    "conversations_opened": 2,
    "calls_total": 6,
    "calls_answered": 6,
    "calls_avg_duration_seconds": 22
  }
```

### Sparkline points for calls

```bash
GET /api/v1/analytics/series?kind=calls_daily&from=2026-08-22&to=2026-09-05
→ { "name": "calls", "points": [{"day":"2026-09-05","value":6}, ...] }
```

### Provider-call telemetry (Meta API log)

Every Meta HTTP round-trip is recorded — request URL, status, latency, error class, response body — searchable per integration:

```bash
GET /api/v1/settings/provider-calls?integration_id=...&status_min=400&limit=50
```

## Gotchas

- **Rollup window**: the runner rolls yesterday + today + tomorrow (local) every 15 minutes to catch tz boundary drift.
- **KPI vs series fallback**: if a day's per-direction detail rows are present but the pan-direction "all" row is missing (older rollups), both KPI and series fold detail rows for that day.
- **Response time p50**: computed over per-day averages, not per-message medians — fast but coarse.
- **Sparkline "max 2" with flat line**: the chart has a floor of 1 with 15% headroom; all-zero data reads as "max 2" — not real data.

## Related skills

- [`nudgeway-inbox`](../nudgeway-inbox/SKILL.md) — every message that lands in the inbox becomes a rollup row within 15 minutes.
- [`nudgeway-integrations`](../nudgeway-integrations/SKILL.md) — provider-call telemetry is the debugging surface when messages / calls fail.
