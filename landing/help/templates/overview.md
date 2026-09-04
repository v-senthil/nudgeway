# Templates overview

WhatsApp templates are the pre-approved message shapes Meta reviews before you can use them. They are the only messages allowed outside the 24-hour customer-service window, and the only way to open an outbound-first conversation. Nudgeway mirrors template state from Meta, lets you draft and submit new ones, and renders them WhatsApp-native in the inbox.

## Lifecycle

```
DRAFT  →  PENDING  →  APPROVED  (send-ready)
                  ↘   REJECTED  (edit + resubmit as new)
                  ↘   PAUSED    (quality gate)
                  ↘   DISABLED  (bad quality; not usable)
```

- **DRAFT** — persisted locally, never sent to Meta. Edit freely.
- **PENDING** — submitted; Meta reviewing. Usually minutes; can be hours.
- **APPROVED** — appears in the composer's template picker and in `postMessagesSend` with `type: template`.
- **REJECTED** — Meta returned a reason. Fix, create a new draft (name + language + integration_id is the natural key — you can't re-submit the same triple), submit.
- **PAUSED / DISABLED** — quality-driven. Templates that generate too much user friction get paused; ignore-rate stays high, they get disabled.

## Categories

- `AUTHENTICATION` — OTPs, login codes. Strict formatting (no marketing language).
- `MARKETING` — promotions, offers. Highest quality bar; opt-out required.
- `UTILITY` — transactional (order updates, appointment reminders). Middle tier.

Category mismatches (e.g. a template with promo copy submitted as `UTILITY`) reject. Set `allow_category_change: true` on create to let Meta re-categorize during review.

## Placeholder syntax

- Positional: `{{1}}`, `{{2}}`, `{{3}}`, ...
- Named: `{{customer_name}}`, `{{order_id}}`, ...

The backend accepts both. Parameter counts + names must match at send time — `postMessagesSend` returns `422 parameter count mismatch` otherwise.

## Analytics lookback

The Meta Analytics tab pulls template-level analytics (sent, delivered, read, click-through) with a **90-day lookback window** (Meta's cap). Older ranges silently truncate to 90 days on Meta's side.

## What ships in this module

| Feature | Page |
|---|---|
| Create a template | [Create a template](/#/templates/create) |
| Submit for Meta review | [Submit for Meta review](/#/templates/submit-for-review) |
| Sync status from Meta | [Sync from Meta](/#/templates/sync-from-meta) |
| Common failure modes | [Troubleshooting](/#/templates/troubleshooting) |

## Related

- [Send a template message](/#/inbox/send-template) — send flow.
- [Meta Analytics tab](/#/analytics/meta-analytics-tab) — 90-day template performance.
- [Connect a WhatsApp integration](/#/integrations/connect-whatsapp) — templates require an integration with the `whatsapp_business_management` scope.
