# Create a template

Templates start as local drafts. You can save a draft and submit later, or create and submit in one step.

## How to use

1. Go to **Settings → Templates** and click **New template**.
2. Pick the target integration (the WhatsApp number the template belongs to).
3. Pick the language. Use Meta's locale codes — `en`, `en_US`, `pt_BR`, `hi`, etc.
4. Pick the category — **AUTHENTICATION**, **MARKETING**, or **UTILITY**.
5. Enter a name. Lowercase letters, numbers, and underscores only; 1 to 512 characters.
6. Build the components:
   - **Header** (optional) — text, image, video, or document.
   - **Body** (required) — the main message. Use `{{1}}`, `{{2}}` for positional placeholders or `{{name}}` for named ones.
   - **Footer** (optional) — short branding line.
   - **Buttons** (optional, up to 3) — URL, Quick Reply, Phone Number, Copy Code, or Voice Call.
7. Click **Save as draft** to persist locally, or tick **Submit for Meta review** and click **Save** to send it straight to Meta.

## Category rules

- **AUTHENTICATION** — OTP shape only. Body must read like `{{1}} is your verification code.` Buttons limited to a single Copy Code button.
- **MARKETING** — promo copy allowed but must include an opt-out affordance.
- **UTILITY** — order confirmations, appointment reminders, shipping updates. Keep the tone transactional; no promo phrasing.

If you're unsure which category fits, tick **Allow Meta to re-categorize** — Meta will move it during review instead of rejecting.

## Troubleshooting

- **"Invalid name" red banner** — the name has an unsupported character. Use only lowercase letters, digits, and underscores. No hyphens, no capitals.
- **"Body is required" banner** — every template needs at least one Body component. Header, footer, and buttons are optional.
- **"Target integration missing"** — the WhatsApp integration you picked was deleted or disabled. Reconnect it in Settings → Integrations.
- **"Permission denied — template management"** — your role doesn't allow managing templates. Ask an admin to grant the templates-manage permission.
- **Submit inline failed with a Meta reason** — the exact message from Meta appears in the error toast. Read it, edit the copy, and try again. Common causes: promotional wording in a UTILITY template, unbalanced placeholders, or a reserved word in the body.

## Related

- [Templates overview](#/templates/overview) — lifecycle and categories.
- [Submit for Meta review](#/templates/submit-for-review) — submit a draft later.
- [Troubleshooting](#/templates/troubleshooting) — rejection reasons.
