# Conversation list

The conversation list is the left pane of the Inbox and the entry point for every reply. Rows are grouped by status (open, pending, resolved), sorted newest-activity-first, and show the contact name, avatar, last-message preview, and unread count.

## How to use

1. Click a row to load its thread into the middle pane. Opening a conversation automatically marks its unread inbound messages as read (blue ticks on the customer's phone).
2. Use the filter chips at the top of the pane to narrow by status, assignee, or channel. Filters combine.
3. Type in the search box to find a contact by name or phone. Search is scoped to your organization.

Group conversations show the group subject instead of a contact name and avatar.

## Troubleshooting

- **The list is empty even though a customer messaged you** — no WhatsApp integration is connected yet, or Meta hasn't been told to send you `messages` events. Go to Settings → Integrations and confirm your integration is connected, then use [Push webhook to Meta](#/integrations/webhook-setup) to register the callback.
- **You see a "session expired" toast when clicking a row** — your login cookie timed out. Log out and back in.
- **A red "action failed" toast appears when opening a conversation** — refresh the page. Your browser lost the security token; the reload will fetch a fresh one automatically.
- **New messages don't appear live** — your real-time connection dropped. Look for a "reconnecting" indicator; if you don't see one in a few seconds, refresh the page.
- **Group conversations show a blank name** — this is expected. The subject line renders in place of a contact name; scroll the right pane to see group members.

## Related

- [Inbox overview](#/inbox/overview) — three-pane layout and real-time behaviour.
- [Send a text message](#/inbox/send-text) — the composer's default path.
- [Mark messages as read](#/inbox/mark-read) — blue-tick semantics.
