# Phone number details + QR

A read-only view of Meta's metadata for the phone number on this integration, plus a customer-facing QR code that opens a WhatsApp chat with your business.

## Fields

| Field | Notes |
|---|---|
| **Phone Number ID** | The numeric id Meta uses to reference this number. |
| **Display phone number** | The E.164 number your customers see, e.g. `+1 555 123-4567`. |
| **Verified name** | The Meta-approved business name shown to customers. |
| **Status** | Meta's phone-number status (Connected, Migrated, Pending, ...). |
| **Quality rating** | Green, Yellow, or Red. See below. |
| **Country code** | Two-letter country code. |
| **Verification status** | Whether Meta's SMS / voice verification is complete. |
| **Account mode** | Sandbox or live. |
| **Host platform** | Always "Cloud API" for Nudgeway. |
| **Messaging limit tier** | How many unique customers you can message per day (Tier 50, Tier 1K, Tier 10K, Tier 100K, Unlimited). Meta raises this automatically as your volume grows. |
| **Official Business Account** | Whether the green checkmark badge is granted. Feeds the [OBA](#/integrations/oba-status) section. |

## How to use

1. Click **Settings** -> **Integrations** and pick the integration.
2. Open the **Phone number** section.
3. Review the fields.
4. Below the fields, right-click the QR code and choose **Save image** to hand it out to customers on posters, receipts, business cards, or anywhere physical.

## Troubleshooting

- **The panel is blank or the fields are all empty** — Meta says this Phone Number ID isn't part of the WABA. Double-check the Phone Number ID against the Meta App Dashboard -> WhatsApp -> API Setup page. If it's wrong, delete the integration and re-create it with the correct value.
- **Quality rating is Red** — customers are marking your messages as spam. Slow down outbound sends, review your template quality, and stop sending to unengaged contacts. Meta will throttle a Red number.
- **Messaging limit tier is low** — Meta raises this automatically as you deliver more messages with good quality. Nothing to click; keep quality up and volume will unlock.
- **QR code isn't rendering** — the QR is generated from the display phone number field. If that field is empty (see the first bullet), the QR won't draw either. Fix the Phone Number ID first.

## Related

- [Integrations overview](#/integrations/overview)
- [OBA status](#/integrations/oba-status)
- [Meta Analytics tab](#/analytics/meta-analytics-tab)
