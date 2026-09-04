# Templates troubleshooting

Common template problems and what you can do to fix them yourself. When the on-screen message isn't enough, an admin can open the [Provider calls log](#/audit-telemetry/provider-calls) for the full Meta response.

## Meta rejected the template

- **"Invalid format"** — the body has stray characters (unbalanced `{{`, unclosed quotes) or an overly long placeholder. Simplify the body and use short placeholders.
- **"Tag content mismatch"** — you submitted as UTILITY but the copy reads like MARKETING (or vice versa). Duplicate the template under the correct category, or tick **Allow Meta to re-categorize** on [create](#/templates/create).
- **"Abusive content"** — Meta flagged the copy as misleading, overly promotional in a utility, or containing prohibited words. Rewrite the body from scratch; resubmitting the same wording will fail again.
- **"Invalid variables"** — placeholders skip numbers (for example `{{1}} {{3}}` with no `{{2}}`), or a header has more than one variable. Renumber so they run 1, 2, 3 without gaps.
- **"Non-existent template"** — you referenced a template Meta doesn't have. Click [Sync from Meta](#/templates/sync-from-meta) to reconcile.

## Category mismatch

- Submitting UTILITY and getting a mismatch usually means the copy has promotional language. Move it to MARKETING or drop the promo phrasing.
- When **Allow Meta to re-categorize** is ticked, Meta moves the template silently — check the template's badge after review, not before.
- AUTHENTICATION bodies must match the OTP shape exactly. Extra prose or greetings will reject.

## Duplicate name

- **"Duplicate template" on create** — the combination of integration, name, and language must be unique on Meta's side. You can't resubmit the same triple with a different body. Options:
  - Rename: `order_shipped` → `order_shipped_v2`.
  - Pick a different language: `en` → `en_US` (they're separate templates on Meta's side).
  - Delete the old one on Meta first (deletion from within Nudgeway lands in a later release; for now, rename is easiest).

## Sync and status

- **PENDING for hours** — click [Sync from Meta](#/templates/sync-from-meta). Meta occasionally forgets to push the final status.
- **APPROVED locally but PAUSED in the WhatsApp Manager** — the last sync ran before the quality gate tripped. Sync again.
- **"Can I resurrect a DISABLED template?"** — no. Meta has stopped accepting sends against it. Create a fresh template with a better shape.

## Permission

- **"Permission denied — template management"** — your role doesn't allow managing templates. Ask an admin to grant it in Settings → Users.
- **"Permission denied — template read"** — very rare; read is bundled with most default roles. Ask an admin.

## Send-time errors

- **"Template not found" when sending** — the template isn't APPROVED for this integration any more. Click [Sync from Meta](#/templates/sync-from-meta) and re-check.
- **"Parameter count mismatch"** — the number of values you filled doesn't match the placeholders in the approved body. Count `{{1}}`, `{{2}}` in the body and match exactly.
- **"Language rejected"** — the locale doesn't match the template's approved language. Pick a different template whose language matches, or resubmit under the right locale.

## Related

- [Templates overview](#/templates/overview) — lifecycle.
- [Create a template](#/templates/create) — name and category rules.
- [Send a template message](#/inbox/send-template) — send-time flow.
