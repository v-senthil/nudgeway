# Create a template

Templates start as local `DRAFT` rows. You can save a draft and submit later, or create + submit in the same request via the `submit: true` flag.

## How to use

1. Settings → Templates → **New template**.
2. Pick the target integration and language (Meta locale, e.g. `en`, `en_US`, `pt_BR`).
3. Pick the category — `AUTHENTICATION`, `MARKETING`, or `UTILITY`.
4. Enter a name — lowercase alphanumeric + underscore, 1-512 chars.
5. Build the components — `HEADER` (text / image / video / document), `BODY` (required), `FOOTER`, and up to 3 `BUTTONS` (`URL`, `QUICK_REPLY`, `PHONE_NUMBER`, `COPY_CODE`, `VOICE_CALL`).
6. Save as draft, or tick **Submit for Meta review** to send it immediately.

## API

**operationId**: `createTemplate`

```
POST /api/v1/templates
```

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/templates' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
    "name": "order_shipped",
    "language": "en",
    "category": "UTILITY",
    "components": [
      {
        "type": "BODY",
        "text": "Hi {{1}}, your order {{2}} shipped this morning."
      },
      {
        "type": "FOOTER",
        "text": "Acme Co."
      },
      {
        "type": "BUTTONS",
        "buttons": [
          { "type": "URL", "text": "Track", "url": "https://acme.co/track/{{1}}" }
        ]
      }
    ],
    "submit": true,
    "allow_category_change": false
  }'
```

Response is a full `Template` object. If `submit: true`, the row advances to the provider-reported status (usually `PENDING`).

## MCP

Call the `createTemplate` tool with:

```json
{
  "body": {
    "integration_id": "<integration-ULID>",
    "name": "<lowercase_name>",
    "language": "<meta-locale>",
    "category": "AUTHENTICATION | MARKETING | UTILITY",
    "components": [ ... ],
    "submit": true
  }
}
```

## Category rules

- `AUTHENTICATION` — OTPs only. Body must be `{{1}} is your verification code.` shape. Buttons limited to a single `COPY_CODE`.
- `MARKETING` — promo copy allowed. Requires an opt-out affordance.
- `UTILITY` — order confirmations, appointment reminders, shipping updates. No promo language.

Set `allow_category_change: true` if you're unsure and want Meta to re-categorize during review (Meta will move it, e.g., `UTILITY → MARKETING`, without rejecting).

## Troubleshooting

- **`422 invalid name`** — must match `^[a-z][a-z0-9_]{0,511}$`. No hyphens, no uppercase.
- **`422 missing body`** — every template needs at least one `BODY` component. Header / footer / buttons are optional.
- **`424 target integration missing`** — the `integration_id` doesn't exist or was disabled.
- **`403 missing templates.manage`** — the current user's role doesn't grant template management. Ask an admin.
- **`502` from provider on `submit: true`** — Meta rejected inline. Check the response `detail` and re-check the [Provider calls log](/#/audit-telemetry/provider-calls).

## Related

- [Templates overview](/#/templates/overview) — lifecycle + categories.
- [Submit for Meta review](/#/templates/submit-for-review) — submit a DRAFT later.
- [Troubleshooting](/#/templates/troubleshooting) — rejection reasons.
