# Inbox overview

The Inbox is where you handle every live WhatsApp conversation for your organization. It has three panes: the left lists conversations, the middle shows the selected thread with WhatsApp-native bubbles, and the right shows the contact plus the current session context. Everything is scoped to your organization automatically — you cannot see another tenant's data.

## The three panes

- **Left — Conversation list.** Grouped by status (open, pending, resolved). Filter by contact, session, or the assigned agent using the chips at the top.
- **Middle — Thread and composer.** Shows the selected conversation newest-first. The composer at the bottom sends text, media, or an approved template.
- **Right — Contact and session drawer.** Shows the contact profile, recent orders or tickets (when a CRM integration is wired), and the raw session metadata.

## Real-time updates

The inbox stays live automatically. New inbound messages appear without a refresh, delivery ticks flip in real time, and incoming calls raise the [inbound call popup](#/calls/inbound-call). If your network drops, the client reconnects on its own and re-loads the open conversation so you never miss a gap.

## How the entities relate

Contact, session, conversation, message, and ticket are all separate. One contact can have many sessions; one session can have many conversations; one conversation has many messages. The list shows conversations — drill through the right pane to jump to the underlying session or ticket.

## What you can do here

| Task | Page |
|---|---|
| List and filter conversations | [Conversation list](#/inbox/conversations) |
| Send a text reply | [Send a text message](#/inbox/send-text) |
| Send image, video, or document | [Send media](#/inbox/send-media) |
| Send a template (outside the 24-hour window) | [Send a template message](#/inbox/send-template) |
| Blue-tick outstanding inbound messages | [Mark messages as read](#/inbox/mark-read) |
| Common failure modes | [Troubleshooting](#/inbox/troubleshooting) |

## Related

- [Templates overview](#/templates/overview) — required for replies outside the 24-hour window.
- [Calls overview](#/calls/overview) — call state transitions appear inline in the same thread.
- [Connect a WhatsApp integration](#/integrations/connect-whatsapp) — nothing flows until an integration exists.
