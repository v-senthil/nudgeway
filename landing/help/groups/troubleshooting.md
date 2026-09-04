# Groups troubleshooting

## Sync fails with an "official business account" error

**What you see**: You click **Sync** on the Groups page and get an error toast that mentions "requires official business account". The Groups list stays empty.

**Why**: WhatsApp only exposes the Business Groups feature on numbers with the Official Business Account (OBA) badge.

**What to do**:

1. Open **Settings** -> **Integrations** and click the integration.
2. Open the **OBA** section.
3. If the status is `NOT_APPLIED`, click **Apply**.
4. Wait for the status to become `APPROVED` (usually 1-5 business days). Meta reviews the application; there is nothing further to do until they respond.
5. Once approved, return to the Groups page and click **Sync**. Existing conversations and inbound webhooks will start working automatically the moment Meta flips the number.

## Roster looks stale

**What you see**: A group's member list on the detail panel shows people who have left, or the member count doesn't match what you see in the WhatsApp app.

**Why**: Nudgeway does not yet automatically remove members who left a group. New joiners appear immediately on the next sync; departures stick around as ghosts.

**What to do**:

- Click **Sync** on the Groups page to pick up new joiners.
- Trust the member count shown at the top of the row (that number comes directly from Meta) over the length of the roster list.
- If you need an accurate roster today, cross-check against the WhatsApp mobile app.

## Send to a group fails

**What you see**: The message flips to "failed" with a red indicator inside the group thread.

**What to do**: Hover the failure indicator to see the reason. The common cases are:

- **"24-hour window closed"** — send an approved template instead. See [Send a template message](#/inbox/send-template).
- **"template payload invalid"** — re-pick the template and re-fill any variables.
- **"business account restriction"** — your WABA is throttled or under Meta review. Contact your Meta representative.

If the reason is not clear, an admin can open **Settings** -> **Audit** -> [Meta API execution log](#/audit-telemetry/provider-calls) and filter by `send_message` to see Meta's raw response.

## Sync says "0 groups added or updated"

**What you see**: Sync completes cleanly, but no groups appear.

**Why**: Either the integration is not a WhatsApp integration, or the WABA genuinely has no groups yet.

**What to do**:

- Confirm your business number has actually been added to at least one group inside WhatsApp.
- Confirm you picked the correct integration in the drop-down.

## Sync says "integration not found" or "failed dependency"

**What you see**: A red error banner mentioning the integration being disconnected.

**What to do**:

1. Open **Settings** -> **Integrations**.
2. Confirm the integration status pill is `connected`. If it says `disconnected`, click **Test connection**.
3. If the test fails, re-check your credentials — see [Test the connection](#/integrations/test-connection).

## Related

- [Groups overview](#/groups/overview)
- [OBA status](#/integrations/oba-status)
- [Meta API execution log](#/audit-telemetry/provider-calls)
