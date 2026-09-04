# Phone number details + QR

Read-only view of Meta's phone-number metadata for the integration, plus the customer-facing QR code that opens a chat with your business.

An **empty struct is a valid `200` response** when the Phone Number ID is not (yet) part of the WABA (mid-migration, or the id was mistyped).

## Fields

| Field | Notes |
|---|---|
| `id` | The Phone Number ID (echoes `Integration.config.phone_number_id`). |
| `display_phone_number` | The E.164 display number (e.g. `+1 555 123-4567`). |
| `verified_name` | Meta-approved business name (what customers see). |
| `status` | Meta phone-number status (`CONNECTED`, `MIGRATED`, `PENDING`, …). |
| `quality_rating` | `GREEN` / `YELLOW` / `RED`. |
| `country_code` | ISO 3166-1 alpha-2 (e.g. `US`, `IN`). |
| `country_dial_code` | Numeric country code (e.g. `1`, `91`). |
| `code_verification_status` | Whether the SMS / voice verification is complete. |
| `account_mode` | Sandbox vs live. |
| `host_platform` | Cloud API vs On-Prem. Always `CLOUD_API` for Nudgeway. |
| `messaging_limit_tier` | `TIER_50`, `TIER_1K`, `TIER_10K`, `TIER_100K`, `TIER_UNLIMITED`. |
| `is_official_business_account` | Green-checkmark indicator. Feeds the [OBA](/#/integrations/oba-status) section. |

## How to use

1. Settings → Integrations → the row → **Phone number** section.
2. Read the fields — this is the authoritative Meta view.
3. Below the fields, the QR code renders. Right-click → Save image to hand to customers.

## API

```
GET /api/v1/integrations/{id}/phone-number
```

Response `200`:

```json
{
  "id": "1017392881147...",
  "display_phone_number": "+1 555 123-4567",
  "verified_name": "Acme Support",
  "status": "CONNECTED",
  "quality_rating": "GREEN",
  "country_code": "US",
  "country_dial_code": "1",
  "code_verification_status": "VERIFIED",
  "account_mode": "LIVE",
  "host_platform": "CLOUD_API",
  "messaging_limit_tier": "TIER_1K",
  "is_official_business_account": false
}
```

Requires `integrations.manage`.

## MCP

| operationId | Purpose |
|---|---|
| `getIntegrationPhoneNumber` | Read Meta's phone-number metadata. |

## Troubleshooting

- **Empty object returned** — the Phone Number ID is not part of the WABA. Re-check `Integration.config.phone_number_id` against Meta's API Setup page.
- **`quality_rating: RED`** — customers are marking messages as spam. Slow down outbound sends; check template quality. Meta may throttle you.
- **`messaging_limit_tier` low** — Meta ratchets this up as delivered volume grows. Nothing to do on the Nudgeway side.
- **QR code missing** — the QR rendering is client-side (the browser draws it from the `display_phone_number`). If the field is empty, no QR.

## Related

- [Integrations overview](/#/integrations/overview)
- [OBA status](/#/integrations/oba-status)
- [Meta Analytics tab](/#/analytics/meta-analytics-tab)
