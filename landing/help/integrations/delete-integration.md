# Delete an integration

`deleteIntegration` is a **soft-disconnect**, not a hard delete. It flips the integration's `status` to `disconnected` so downstream workers stop picking it up; the row and the envelope-encrypted credentials remain in place for audit.

## What "soft-disconnect" actually does

- **`Integration.status` → `disconnected`.** The send worker, webhook worker, and analytics rollup skip disconnected integrations.
- **Credentials row preserved.** `integration_credentials` is untouched; the KEK-sealed `access_token` / `app_secret` / `verify_token` are still on disk.
- **Webhook_events history preserved.** Every past delivery still lives in `webhook_events`.
- **Provider_calls log preserved.** Every past Meta round-trip is still in `provider_calls` — [Meta API execution log](/#/audit-telemetry/provider-calls) keeps working.
- **Messages / conversations preserved.** Threads still open in the Inbox as read-only.
- **Meta side is untouched.** Nudgeway does not tell Meta to stop delivering webhooks. If you want Meta to stop calling us, remove the webhook subscription in Meta's console *first*.

## When to actually hard-delete

Reasons you'd want the row and credentials permanently gone:

- Compliance requirement (retention window ended).
- Preparing to re-add the same phone number under a new tenant.

There is **no REST endpoint** for hard delete today. Options:

- CLI (once implemented): `nudgeway-cli integration delete <id> --hard`.
- SQL:

  ```sql
  DELETE FROM integration_credentials WHERE credentials_ref = (
    SELECT credentials_ref FROM integrations WHERE id = ?
  );
  DELETE FROM integrations WHERE id = ?;
  ```

  Note: `webhook_events` / `messages` / `provider_calls` rows carry `integration_id`. Decide whether to keep them (audit) or cascade-delete separately.

## How to use

1. Settings → Integrations → the row → **Disconnect** button.
2. Confirm in the modal.
3. The row now renders with `status: disconnected`. Send workers skip it immediately.

## API

```
DELETE /api/v1/integrations/{id}
```

No body. Response `204 No Content`. Requires CSRF + `integrations.manage`. Additional statuses: `404` if the id is unknown or belongs to another tenant.

## MCP

| operationId | Purpose |
|---|---|
| `deleteIntegration` | Soft-disconnect. Row + credentials preserved. |
| `getIntegration` | Read the current status. |

## Re-connecting

To re-enable a disconnected integration without recreating the row:

1. Currently there's no `PATCH /integrations/{id}` REST endpoint — flip the status via SQL:

   ```sql
   UPDATE integrations SET status = 'active' WHERE id = ?;
   ```

2. Run [Test the connection](/#/integrations/test-connection) to confirm Meta accepts the stored credentials.

## Troubleshooting

- **Meta keeps delivering webhooks after disconnect** — expected. Meta doesn't know we soft-disconnected. Remove the webhook subscription in Meta's console.
- **`404 not found` on delete** — the id belongs to another tenant, or you already deleted it.
- **Messages still showing as `sending`** — the send worker flushed them before the disconnect landed. They'll transition to `failed` on retry.

## Related

- [Integrations overview](/#/integrations/overview)
- [Test the connection](/#/integrations/test-connection)
- [Audit log](/#/audit-telemetry/audit-log)
