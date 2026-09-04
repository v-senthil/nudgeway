# Business profile

The business profile is what customers see when they tap your business name in WhatsApp — about text, description, address, email, profile picture, vertical, and up to two websites. Nudgeway exposes read + write over the provider-agnostic `BusinessProfile` shape; today only the WhatsApp adapter maps into it, one-to-one.

## How to use

1. Settings → Integrations → the row → **Business profile** drawer.
2. Edit fields. Save.
3. The `PUT` response reflects the reconciled state — no second round-trip needed.

## Fields

| Field | Type | Notes |
|---|---|---|
| `about` | string | Short one-liner shown under the business name. |
| `address` | string | Free-form. |
| `description` | string | Longer paragraph. |
| `email` | email | Contact email. |
| `profile_picture_url` | URI | Publicly-reachable image URL. Meta re-hosts it. |
| `vertical` | string | Meta vertical enum (e.g. `RETAIL`, `HEALTH`, `EDU`). |
| `websites` | array of URI | Up to two. |

## API

### Read

```
GET /api/v1/integrations/{id}/business-profile
```

Response `200`:

```json
{
  "about": "24/7 support.",
  "address": "1 Market St, San Francisco",
  "description": "Acme Cloud helps teams ship faster.",
  "email": "hello@acme.example",
  "profile_picture_url": "https://cdn.example.com/pic.jpg",
  "vertical": "PROF_SERVICES",
  "websites": ["https://acme.example"]
}
```

Requires `integrations.manage`.

### Write

```
PUT /api/v1/integrations/{id}/business-profile
Content-Type: application/json

{
  "about": "Now open on Sundays.",
  "websites": ["https://acme.example", "https://status.acme.example"]
}
```

The response is the reconciled profile (Nudgeway performs a follow-up `GET` internally so the drawer can render without an extra round-trip).

Requires CSRF + `integrations.manage`. Additional statuses: `400` invalid JSON, `404` integration not found, `502` Meta rejected the write.

## MCP

| operationId | Purpose |
|---|---|
| `getIntegrationBusinessProfile` | Read the current profile. |
| `updateIntegrationBusinessProfile` | Write + return the reconciled profile. |

## Troubleshooting

- **`502` on write** — Meta rejects unsupported vertical values or `websites` with more than two entries. Check the exact error in [Meta API execution log](/#/audit-telemetry/provider-calls) filtered on `operation=update_business_profile`.
- **Profile picture stuck on old image** — Meta caches heavily on the customer side; the change is server-side within seconds but customer devices can take hours.

## Related

- [Integrations overview](/#/integrations/overview)
- [Business username](/#/integrations/business-username)
- [Phone number details + QR](/#/integrations/phone-number-details)
