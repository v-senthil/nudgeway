# Business profile

The business profile is what your customers see when they tap your business name inside WhatsApp — a short "about" line, a description, an address, a contact email, a profile picture, a business vertical, and up to two websites.

## Fields

| Field | Notes |
|---|---|
| **About** | One-liner shown right under your business name. |
| **Address** | Free-form. |
| **Description** | A longer paragraph. |
| **Email** | Contact email address. |
| **Profile picture URL** | A publicly-reachable image URL. Meta re-hosts it. |
| **Vertical** | Meta's business vertical (e.g. Retail, Health, Education). Pick from the drop-down. |
| **Websites** | Up to two URLs. |

## How to use

1. Click **Settings** -> **Integrations** and pick the integration.
2. Open the **Business profile** drawer.
3. Edit the fields you want to change.
4. Click **Save**. The drawer refreshes with the reconciled state so you can see exactly what Meta stored.

## Troubleshooting

- **Save fails with an error banner** — Meta rejects unsupported vertical values and rejects more than two entries in Websites. Check that the vertical you picked is in the drop-down and that Websites has at most two URLs.
- **Profile picture URL saves fine but the picture doesn't change on the customer's phone** — Meta caches heavily on the customer device. The change is live on Meta's servers within seconds; the phone can take hours to pick it up. Nothing to fix in Nudgeway.
- **A field you edited didn't stick** — Meta silently drops fields it deems invalid. Re-open the drawer to see the current state; if the field is empty, adjust the value (shorter, remove special characters, valid URL) and re-save.

## Related

- [Integrations overview](#/integrations/overview)
- [Business username](#/integrations/business-username)
- [Phone number details + QR](#/integrations/phone-number-details)
