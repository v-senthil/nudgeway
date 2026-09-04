# Mark messages as read

Marking an inbound message as read triggers WhatsApp's blue double-tick on the customer's phone. Nudgeway does this automatically the moment you open a conversation — you rarely need to think about it.

## How it works

- **Open a conversation** — every unread inbound in the newest 50 messages is marked read in one batch. Blue ticks appear on the customer's side within seconds.
- **Batching cap** — if a conversation has more than 50 unread inbounds, scroll further back in the thread; the client marks additional pages as you load them.

## Blue-tick behaviour

- The blue tick only appears on the customer's phone if they have read receipts enabled in their own WhatsApp settings (their choice, not yours). If they've turned read receipts off, you'll see delivered ticks but they'll never turn blue.
- The delivered-to-read transition on **your outbound** bubbles comes from a separate signal that WhatsApp emits when the customer opens the message. That's independent of your marking their inbound as read.

## Troubleshooting

- **You opened the conversation but the blue tick never appears on the customer's phone** — the customer has read receipts turned off in their WhatsApp Settings → Privacy. Nothing to fix on your side.
- **You see a red "Couldn't mark as read" toast** — the WhatsApp integration was disconnected. Reconnect via [Integrations](#/integrations/connect-whatsapp).
- **The read state didn't propagate to another operator's inbox** — their real-time connection dropped. Ask them to refresh.
- **You see a "provider rejected" error banner** — Meta rate-limited the call, or had a transient error. It'll retry automatically; open and re-open the conversation to trigger another attempt.

## Related

- [Conversation list](#/inbox/conversations) — auto-marks read on open.
- [Send a text message](#/inbox/send-text) — outbound send and delivery ticks.
- [Troubleshooting](#/inbox/troubleshooting) — real-time updates and authentication.
