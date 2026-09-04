# Delete an integration

**Disconnect** is a soft delete. Nudgeway stops sending and stops accepting webhooks for the integration, but the row and its encrypted credentials stay on file so audit history keeps working.

## What "disconnect" does

- The status pill flips to **Disconnected**. Nudgeway will not send new messages, pull analytics, or process incoming webhooks for this integration.
- Past messages and conversations are preserved. You can still open those threads in the Inbox; they're read-only.
- The Audit log and the [Meta API execution log](#/audit-telemetry/provider-calls) keep every historical entry for this integration.
- Meta is **not** told to stop. Meta will keep trying to deliver webhooks to your Webhook URL — those deliveries are simply ignored. If you want Meta to actually stop, remove the webhook subscription in your Meta App Dashboard first.

## How to use

1. Click **Settings** -> **Integrations**.
2. Find the integration row you want to disconnect.
3. Click **Disconnect**.
4. Confirm in the modal.

The row now shows a **Disconnected** pill.

## Re-connecting

There is no in-place "reconnect" button today. To bring a disconnected integration back:

1. Delete the row (see below).
2. Recreate the integration with the same credentials via [Connect a WhatsApp integration](#/integrations/connect-whatsapp).
3. Click **Test connection**.

If you need the disconnect to be undone without losing the row (for example, because it's referenced by many audit entries and you want history to stay linked), contact your admin — this requires a database change today.

## Fully removing an integration

Disconnect is intentionally reversible. To permanently delete the integration and its encrypted credentials — for compliance, or because you're re-adding the same phone number under a new workspace — contact your admin. There is no self-serve hard-delete button today.

## Troubleshooting

- **Meta keeps sending webhooks after I disconnect** — expected. Nudgeway ignores them, but you need to remove the webhook subscription in your Meta App Dashboard to stop the deliveries.
- **"Not found" error on Disconnect** — the row was already deleted, or belongs to a different workspace. Refresh the integrations list.
- **Messages I sent right before disconnecting still show "sending"** — the send worker had already dispatched them. They'll flip to "sent" or "failed" once WhatsApp responds, or the retry timer expires.

## Related

- [Integrations overview](#/integrations/overview)
- [Test the connection](#/integrations/test-connection)
- [Audit log](#/audit-telemetry/audit-log)
