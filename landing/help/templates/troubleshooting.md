# Templates troubleshooting

Common template failure modes and fixes. All Meta responses are captured in the [Provider calls log](/#/audit-telemetry/provider-calls) — start there when the message here isn't enough.

## Rejection reasons

- **`INVALID_FORMAT`** — the body has stray characters (unbalanced `{{`, unclosed quotes) or too-long variables. Simplify the body; use short placeholders.
- **`TAG_CONTENT_MISMATCH`** — you submitted as `UTILITY` but the body reads like `MARKETING` (or vice versa). Recreate under the right category, or set `allow_category_change: true` on [create](/#/templates/create).
- **`ABUSIVE_CONTENT`** — Meta flagged the copy as misleading, promotional-in-a-utility, or containing prohibited terms. Rewrite from scratch — resubmitting the same body will fail again.
- **`INVALID_VARIABLES`** — placeholders skip numbers (e.g. `{{1}} {{3}}` with no `{{2}}`), or the header has more than one variable. Renumber sequentially.
- **`NON_EXISTENT_TEMPLATE`** — you referenced a template by a name that isn't on Meta's side. Run [Sync from Meta](/#/templates/sync-from-meta).

## Category mismatch

- Submitted `UTILITY` gets rejected with `TAG_CONTENT_MISMATCH` → the copy has promotional language. Move to `MARKETING`, or drop the promo phrasing.
- Meta silently re-categorizes when `allow_category_change: true` — check the row's `category` after review, not before.
- `AUTHENTICATION` bodies must match Meta's OTP template shape. Anything else (greetings, extra prose) rejects.

## Duplicate name

- **`422 duplicate template`** on create — `(integration_id, name, language)` is the natural key on Meta's side. You cannot resubmit the same triple with a different body. Options:
  - Delete the old row locally and Meta-side, then create fresh (Meta-side delete lands in a later phase — for now, rename).
  - Rename: `order_shipped` → `order_shipped_v2`.
  - Change the language: `en` → `en_US` (technically a different template on Meta's side).

## Sync + status

- **`PENDING` for hours** — Meta occasionally doesn't push the terminal-state webhook. Force [Sync from Meta](/#/templates/sync-from-meta).
- **`APPROVED` locally, `PAUSED` on Meta** — the last sync ran before the quality gate tripped. Re-sync.
- **`DISABLED` — can I resurrect?** — no. Meta has stopped accepting sends against it. Create a new template with a better shape.

## Permission

- **`403 missing templates.manage`** — the user's role doesn't include template management. Admin can grant under Settings → Users.
- **`403 missing templates.read`** — same, for read-only. Rare — this permission is bundled with most default roles.

## Send-time errors

- **`422 template not found`** on `postMessagesSend` — template isn't `APPROVED` for this integration. Run [Sync from Meta](/#/templates/sync-from-meta), then re-check `status`.
- **`422 parameter count mismatch`** — you passed 2 parameters, the body has 3 placeholders (or vice versa). Count `{{1}}`, `{{2}}`, ... in the body and match.
- **`422 language rejected`** — the `language` sent doesn't match the template's approved locale. Use the exact locale Meta approved (e.g. `en_US`, not `en-us`).

## Related

- [Templates overview](/#/templates/overview) — lifecycle.
- [Create a template](/#/templates/create) — name + category rules.
- [Send a template message](/#/inbox/send-template) — send-time parameter shape.
