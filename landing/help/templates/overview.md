# Templates overview

WhatsApp templates are the pre-approved message shapes Meta reviews before you can use them. They are the only messages allowed outside the 24-hour customer-service window, and the only way to start a new outbound conversation. Nudgeway mirrors template state from Meta, lets you draft and submit new ones, and renders them WhatsApp-native in the Inbox.

## Lifecycle

```
DRAFT  →  PENDING  →  APPROVED  (ready to send)
                  ↘   REJECTED  (edit and resubmit as a new template)
                  ↘   PAUSED    (Meta quality gate)
                  ↘   DISABLED  (unusable)
```

- **DRAFT** — saved locally, never sent to Meta. Edit freely.
- **PENDING** — submitted to Meta and awaiting review. Usually minutes, occasionally hours.
- **APPROVED** — appears in the composer's template picker and is sendable.
- **REJECTED** — Meta returned a reason. Fix and create a new draft (the same name plus language plus integration is unique — you can't resubmit the same triple).
- **PAUSED** — Meta throttled sends because quality dropped. Fix the copy and iterate.
- **DISABLED** — Meta stopped accepting sends against it. Start a new template with a better shape.

## Categories

- **AUTHENTICATION** — OTPs and login codes. Strict formatting — no marketing language.
- **MARKETING** — promotions and offers. Highest quality bar; must include an opt-out.
- **UTILITY** — transactional (order updates, appointment reminders). Middle tier.

If the copy doesn't match the category (for example, promo language in a UTILITY template), Meta rejects it. When you're unsure, tick **Allow Meta to re-categorize** on the create form — Meta will move it during review instead of rejecting.

## Placeholders

- **Positional** — `{{1}}`, `{{2}}`, `{{3}}`.
- **Named** — `{{customer_name}}`, `{{order_id}}`.

Both work. At send time, every placeholder in the body must have a value.

## Analytics lookback

The [Meta Analytics tab](#/analytics/meta-analytics-tab) shows per-template counts (sent, delivered, read, click-through) for the last **90 days**. Older ranges are silently trimmed by Meta.

## What you can do here

| Task | Page |
|---|---|
| Create a template | [Create a template](#/templates/create) |
| Submit for Meta review | [Submit for Meta review](#/templates/submit-for-review) |
| Refresh statuses from Meta | [Sync from Meta](#/templates/sync-from-meta) |
| Common failure modes | [Troubleshooting](#/templates/troubleshooting) |

## Related

- [Send a template message](#/inbox/send-template) — send flow.
- [Meta Analytics tab](#/analytics/meta-analytics-tab) — 90-day template performance.
- [Connect a WhatsApp integration](#/integrations/connect-whatsapp) — templates require an integration with management scope.
