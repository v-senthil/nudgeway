# Groups

A **Group** is a WhatsApp Business group chat that Nudgeway tracks alongside your 1:1 conversations. A group appears as a single thread in the Inbox no matter how many participants have written to it, and it lives beside Contacts, Sessions, Conversations, Messages, and Tickets as its own kind of thing.

## OBA-only

WhatsApp only exposes the Business Groups feature on numbers that Meta has granted the **Official Business Account (OBA)** badge. Until your integration is OBA-approved:

- Clicking **Sync** on the Groups page produces an error toast that mentions "requires official business account".
- The Groups list on the page stays empty.

Open Settings -> Integrations -> **OBA** for that integration. If the status shows `NOT_APPLIED`, click **Apply** and wait for `APPROVED` (typically 1-5 business days). Once approved, come back to the Groups page and click **Sync** again.

## What Nudgeway keeps per group

Each group row on the page shows:

- The subject line (may be blank) and an optional description.
- A member count.
- An "Admin" chip if your business number has admin rights on the group. Only admins can add or remove participants, or reset the invite link. Non-admin groups are read-only from the composer's perspective.

Members appear individually once they message the group; a participant who has never messaged your business shows up on the roster but not yet in your contacts list.

## Inbox behaviour

When someone messages the group, the message lands in the group thread in your Inbox rather than a 1:1 chat, even though the sender is a normal contact. Replying from that thread sends to the whole group.

## Related

- [List + sync groups](#/groups/list-sync)
- [Send to a group](#/groups/send-to-group)
- [Groups troubleshooting](#/groups/troubleshooting)
- [OBA status](#/integrations/oba-status)
