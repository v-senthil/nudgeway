# Send a template message

Templates are the only messages WhatsApp allows outside the 24-hour customer-service window. They are pre-approved by Meta and rendered on the recipient's phone with the exact layout (header, body, footer, buttons) that was approved.

## When to use a template

- Any reply more than 24 hours after the customer's last inbound message.
- The first message when you're starting an outbound conversation (marketing, transactional notifications).
- Any message where the recipient needs a tappable button (Quick Reply, URL, Phone Number, Copy Code, Voice Call).

If you try to send plain text outside the 24-hour window, the composer switches automatically to the template picker.

## How to use

1. Open a conversation whose 24-hour window has expired. The composer shows a **Send template** button instead of the text box.
2. Click **Send template**. A picker opens listing every approved template for this integration.
3. Pick a template. If it has variables (placeholders like `{{1}}`, `{{2}}`, or named ones like `{{customer_name}}`), form fields appear — one per variable.
4. Fill in every variable. Empty fields will fail on send.
5. Click **Send**. The bubble renders in the thread with the full WhatsApp-native layout — header, body, footer, and button chips.

## Troubleshooting

- **The picker says "No approved templates for this integration"** — no template has been approved yet for this WhatsApp number's default language. Go to [Templates](#/templates/overview) and either run [Sync from Meta](#/templates/sync-from-meta) to pull existing ones, or [create a new one](#/templates/create).
- **You see "Template not found" after clicking Send** — the template was deleted or paused between opening the picker and clicking Send. Refresh the picker and pick another one.
- **You see "Variable count doesn't match the template"** — the template's approved body has a different number of placeholders than the form is asking for. Reopen the picker; if it repeats, run [Sync from Meta](#/templates/sync-from-meta) to refresh the local template shape.
- **The message arrives on the recipient's phone as plain text with `{{1}}` visible** — the template wasn't sent as a template. Refresh and re-send from the template picker (not the text box).
- **The buttons the template was designed with don't appear on the recipient's phone** — the template's approved version doesn't include those buttons. Check the template's current state in [Templates](#/templates/overview); if the buttons are missing from the approved shape, submit a new template with the buttons.
- **You see "Language rejected"** — the template's approved locale doesn't match what's being sent. Pick a different template or resubmit under the correct locale.

## Related

- [Templates overview](#/templates/overview) — lifecycle and review.
- [Create a template](#/templates/create) — build a new one.
- [Send a text message](#/inbox/send-text) — the in-window path.
