# List + sync groups

The Groups page in Settings shows every WhatsApp group Nudgeway knows about for your organization. **Sync** pulls the current roster from Meta so new groups, new members, and updated subjects show up.

## How to use

1. Click **Settings** -> **Groups**.
2. If you run more than one WhatsApp integration, pick one from the integration drop-down at the top. Otherwise the drop-down is pre-selected.
3. Use the search box to filter by subject; the list updates as you type.
4. Click **Sync** to fetch the latest groups and members from Meta. A toast at the bottom-right reports how many groups and members were added or updated.
5. Click any row to open its detail panel, which shows the full member roster (name, WhatsApp id, and role: member / admin / superadmin).

Sync is safe to run as often as you want. Groups that already exist are refreshed in place, not duplicated.

## When to sync

- Right after your integration is granted OBA (see [OBA status](#/integrations/oba-status)).
- Whenever you have added the business number to a new group in WhatsApp.
- Whenever you notice the member count on the page looks stale.

## Troubleshooting

- **Sync toast says "0 groups added or updated"** — either your integration is not OBA-approved yet, or your WABA has no groups. Check the OBA status pill on the integration.
- **Error toast mentions "requires official business account"** — apply for OBA on the integration and wait for approval, then retry.
- **Error toast says "integration not found"** — the integration is disconnected. Re-run [Test the connection](#/integrations/test-connection) and try Sync again.
- **Roster shows people who have left the group** — Nudgeway does not yet remove departed members automatically. A rejoin re-opens the row. In the meantime, treat the member count on the row header (which comes from Meta) as the source of truth.

## Related

- [Groups overview](#/groups/overview)
- [Send to a group](#/groups/send-to-group)
- [Meta API execution log](#/audit-telemetry/provider-calls)
