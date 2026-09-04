# Call permissions

WhatsApp users control which businesses can call them. Before you place an outbound call, Nudgeway checks the recipient's current permission. If they haven't granted one, you send an interactive permission request and wait for them to tap Accept.

## The three states

| State | Meaning | Can you call? |
|---|---|---|
| **Permanent** | User granted permission indefinitely | Yes, whenever you want |
| **Temporary** | User granted a time-limited permission | Yes, until the expiration time shown on the button |
| **No permission** | Never granted, or a previous grant expired | No — send a permission request first |

## Check the current state

Open the contact's conversation. The **Call** button in the right pane shows a coloured chip with the current state — green for permanent, amber for temporary (with the expiry time), grey for no permission.

## Request permission

When the state is **No permission**:

1. Click the **Request permission** chip on the Call button (or the **Request** action in the composer's overflow menu).
2. A dialog appears asking for a short prompt. Type a plain-English sentence explaining why you want to call — for example, "May we call you to discuss your recent order?"
3. Click **Send request**. WhatsApp shows the customer an interactive prompt with Accept and Decline buttons.
4. When they tap Accept, a `call_permission_reply` bubble arrives in the thread. The Call button turns green within a few seconds. When they tap Decline, the state stays as No permission — you can't retry immediately (WhatsApp rate-limits these).

Every permission request is recorded in the [Audit log](#/audit-telemetry/audit-log) for compliance.

## Troubleshooting

- **The Call button shows the wrong state** — refresh the page; the real-time update may have been missed.
- **"Permission denied — call read"** — your role doesn't allow reading call state. Ask an admin.
- **"Integration not found"** — the WhatsApp integration was deleted. Reconnect it in Settings → Integrations.
- **"This deployment doesn't support call permissions"** — the WhatsApp adapter in this build is older than the call-permissions feature. Ask an admin to upgrade Nudgeway.
- **"Provider error" when checking state** — Meta had a transient error. Wait 30 seconds and re-open the conversation.
- **Customer tapped Accept but the state still shows No permission** — the reply is delayed. Wait 30 to 60 seconds and refresh; if it's still stuck, ask an admin to verify webhook subscriptions include `call_settings_update`.

## Related

- [Outbound calls](#/calls/outbound-call) — placing the call once permission exists.
- [Calls overview](#/calls/overview) — full call lifecycle.
- [Call settings](#/integrations/call-settings) — per-integration call hours and defaults.
