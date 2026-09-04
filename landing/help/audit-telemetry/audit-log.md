# Audit log

An append-only trail of every action taken inside your workspace. Every mutation writes a row here: creating an integration, sending a message, marking a conversation read, uploading an attachment, logging in, revoking an API token.

## Columns you see

| Column | Notes |
|---|---|
| **Occurred at** | Timestamp of the action. |
| **Actor** | Which user did it. Empty for system-driven actions like the analytics rollup. |
| **Action** | The verb — see the list below. |
| **Resource** | The kind of thing acted on (integration, message, conversation, attachment, session) and its id. |
| **IP** | Client IP address the action came from. |
| **Details** | Free-form context added by the action (e.g. "reason: operator_soft_disconnect"). |

## How to use

1. Click **Settings** -> **Audit** -> **Audit log** tab.
2. Use the filters at the top:
   - **Resource type** and **resource id** together to see all actions on one specific thing (both required).
   - **Action** to see all instances of one verb, e.g. every `integration.disconnected`.
   - **Actor** to see everything one user did.
   - **Time range** with the date picker.
3. Rows show newest-first. Scroll to load more.

## Actions you'll commonly see

- **Integrations**: `integration.created`, `integration.updated`, `integration.tested`, `integration.disconnected`, `integration.webhook_pushed`.
- **Messages**: `message.sent`, `message.marked_read`, `conversation.marked_read`.
- **Attachments**: `attachment.uploaded`.
- **Auth**: `user.logged_in`, `user.logged_out`, `session.expired`.
- **API tokens**: `api_token.created`, `api_token.revoked`.

## Troubleshooting

- **Filter shows nothing when you expect results** — if you're filtering by resource id, make sure resource type is also set. Filtering by id alone won't work; pick a resource type from the drop-down first.
- **Pagination stops earlier than expected** — cursors depend on the current filter shape. If you change any filter mid-scroll, the "load more" cursor resets. Re-scroll from the top after a filter change.
- **You expected to see an entry for an action but it isn't there** — a small number of legacy code paths don't yet write to the audit log. Contact your admin if a critical action is missing; it can be added.

## Related

- [Audit & Meta telemetry overview](#/audit-telemetry/overview)
- [Meta API execution log](#/audit-telemetry/provider-calls)
- [API token usage log](#/api-tokens/usage-log-metrics)
