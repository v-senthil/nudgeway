# Meta Analytics tab (WABA proxy)

The Meta Analytics tab is a live proxy over the WhatsApp Business Cloud analytics APIs. Every panel is one Meta HTTP round-trip scoped to the integration you pick in the top-left drop-down. Requires `analytics.read`.

Three sub-sections ship today:

- **Messaging** — sent + delivered counts.
- **Calls** — count, cost, average duration.
- **Pricing** — cost + volume split by category, type, and volume tier.

Two more sub-sections (**Conversations** and **Templates**) are wired end-to-end on the backend (`getMetaConversationAnalytics`, `getMetaTemplateAnalytics`) but are not currently rendered in the tab.

## How to use

1. **Analytics** → **Meta Analytics** tab.
2. **Integration picker** in the top-left — pick one of your WhatsApp integrations. The picker's `display_phone_number` is auto-filled as the default `phone_numbers` filter.
3. **Quick range** — 7d / 30d / 90d buttons. Custom ranges via the date fields.
4. **Granularity** — `DAY` / `HALF_HOUR` / `MONTH` (Messaging); `DAILY` / `HALF_HOUR` / `MONTHLY` (Calls, Pricing).
5. **Per-integration phone_numbers filter** — free-text override; comma-separated E.164 numbers without `+`.

## API

All endpoints require the integration ULID in the path and a `since`/`until` RFC 3339 range. All are `GET` and support both `sessionCookie` and `apiKey` auth.

### Messaging

```
GET /api/v1/integrations/{id}/meta-analytics/messaging
  ?since=2026-08-29T00:00:00Z
  &until=2026-09-05T00:00:00Z
  &granularity=DAY
  &phone_numbers=15551234567
```

Optional filters: `product_types` (comma-separated `0,2,100`), `country_codes` (ISO 3166-1 alpha-2).

Response `200` mirrors Meta's `analytics.data_points[]` — each point has `start`, `end` (UNIX seconds), `sent`, `delivered`.

### Calls

```
GET /api/v1/integrations/{id}/meta-analytics/calls
  ?since=…&until=…&granularity=DAILY
  &phone_numbers=…&directions=USER_INITIATED,BUSINESS_INITIATED
  &dimensions=phone,direction,country
  &metric_types=COUNT,COST,AVERAGE_DURATION
```

Response fields per point: `count`, `cost`, `average_duration` (seconds), plus optional `phone_number`, `country`, `direction`.

### Pricing

```
GET /api/v1/integrations/{id}/meta-analytics/pricing
  ?since=…&until=…&granularity=DAILY
  &phone_numbers=…&country_codes=…
  &metric_types=COST,VOLUME
  &pricing_types=FREE_CUSTOMER_SERVICE,FREE_ENTRY_POINT,REGULAR
  &pricing_categories=AUTHENTICATION,MARKETING,SERVICE,UTILITY,…
  &dimensions=COUNTRY,PHONE,PRICING_CATEGORY,PRICING_TYPE,TIER
```

Response fields per point: `country`, `phone_number`, `tier` (`"<LOWER>:<UPPER>"` or `"0:MAX"`; omitted for free messages), `pricing_type`, `pricing_category`, `volume`, `cost`.

### Deployed but not currently in the tab

```
GET /api/v1/integrations/{id}/meta-analytics/conversations
GET /api/v1/integrations/{id}/meta-analytics/templates?template_ids=…
```

`template_ids` is capped at 10 per request by Meta; `enable_template_analytics` must be set on the WABA.

## MCP

| operationId | Purpose |
|---|---|
| `getMetaMessagingAnalytics` | Sent + delivered counts. |
| `getMetaCallAnalytics` | Call count, cost, average duration. |
| `getMetaPricingAnalytics` | Pricing tier + category + cost. |
| `getMetaConversationAnalytics` | Conversation counts + cost. |
| `getMetaTemplateAnalytics` | Per-template sent / delivered / read / clicked / cost. |

## Troubleshooting

- **Cards blank** — the integration picker has not resolved yet, or the WABA has no data in the range. Widen to the 90-day preset.
- **`400 invalid_query`** — check that `since` < `until` and both are RFC 3339 (`2026-09-01T00:00:00Z`).
- **`502 provider_error`** — Meta rejected the call; open [Meta API execution log](/#/audit-telemetry/provider-calls) to see the exact reason.
- **Pricing tier missing** — expected for free tier / free-entry-point / free-customer-service pricing types. Not an error.
- **Integration picker won't switch** — see [Analytics troubleshooting](/#/analytics/troubleshooting).

## Related

- [Analytics overview](/#/analytics/overview)
- [Nudgeway tab](/#/analytics/nudgeway-tab)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
