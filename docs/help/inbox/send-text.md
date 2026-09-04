# Send a text message

The composer at the bottom of the middle pane is the primary way to send plain-text replies. Your message appears in the thread immediately and updates its delivery status live as WhatsApp confirms each step.

## How to use

1. Click a conversation in the [list](#/inbox/conversations).
2. Type your message in the composer at the bottom of the middle pane.
3. Press **Send** (or hit `Enter` — `Shift+Enter` inserts a newline).

Your bubble renders with a spinning tick while it's being sent, one grey tick once WhatsApp accepts it, two grey ticks when it's delivered to the recipient's phone, and two blue ticks once the recipient opens it. The transitions arrive automatically — no refresh needed.

If the recipient's message contains a URL, WhatsApp shows a link preview automatically when the URL is on its own line.

## Troubleshooting

- **You see a red banner "Message can't be sent — the 24-hour window has expired"** — WhatsApp only allows plain text within 24 hours of the customer's last inbound message. Click **Send template** in the composer and pick an approved [template](#/inbox/send-template) instead.
- **You see "Conversation not found" toast** — the conversation was archived or moved. Click back to the list and re-open it from there.
- **You see "This conversation has no active integration"** — the WhatsApp integration for this conversation was deleted or disabled. Go to Settings → Integrations and reconnect via [Connect a WhatsApp integration](#/integrations/connect-whatsapp).
- **Your bubble stays on the spinning tick for more than a few seconds** — your send didn't leave the queue. Wait 15 seconds; if it still hasn't moved, refresh the page. If the problem persists after refresh, an admin can check the [Provider calls log](#/audit-telemetry/provider-calls).
- **You see two copies of the same message in the thread** — refresh once. If it happens again, report it — the client should collapse duplicates automatically.

## Related

- [Send media](#/inbox/send-media) — image, video, or document.
- [Send a template message](#/inbox/send-template) — outside the 24-hour window.
- [Mark messages as read](#/inbox/mark-read) — blue-tick the customer's inbound.
