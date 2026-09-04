# Inbox troubleshooting

Common inbox problems and what you can do to fix them yourself. If the fix here doesn't work, ask an admin to check the [Provider calls log](#/audit-telemetry/provider-calls) — every WhatsApp call is captured there with the exact request and response.

## 24-hour window

- **"Message can't be sent — the 24-hour window has expired"** — WhatsApp allows plain text only within 24 hours of the customer's last inbound message. Click **Send template** in the composer and pick an approved [template](#/inbox/send-template).
- **The composer is greyed out even after a fresh inbound message** — refresh the page. Your session missed the update that would have re-enabled it.
- **The template picker says "No approved templates"** — this integration has no approved templates for its default language. Go to [Templates](#/templates/overview) and either [Sync from Meta](#/templates/sync-from-meta) or [create a new template](#/templates/create).

## Authentication

- **You see a "Session expired" toast** — your login timed out (default is 24 hours). Log out and back in.
- **You see a "Permission denied" toast on sending** — your role doesn't allow sending messages. Ask an admin to grant the messaging permission from Settings → Users.
- **A red "action failed" toast on any button click** — refresh the page. Your browser lost its security token; reloading fetches a fresh one.

## Real-time updates dropped

- **Live updates stop; new messages don't appear** — your real-time connection dropped. If you don't see a "reconnecting" indicator within a few seconds, refresh the page. On refresh the client re-fetches the open conversation so nothing is lost.
- **Frequent disconnects when using a corporate VPN or proxy** — some proxies close idle connections aggressively. Ask your IT team to whitelist WebSocket connections to the Nudgeway host.
- **Delivery ticks never advance past one grey tick** — Meta isn't sending status updates to Nudgeway. Ask an admin to check webhook subscriptions via [Push webhook to Meta](#/integrations/webhook-setup).

## Attachment upload

- **Red toast "File too large"** — files must be 16 MiB or smaller. Compress or split.
- **"Meta couldn't fetch the file"** — for local development, the public tunnel is down. For a hosted deployment, refresh and try again; if it still fails, ask an admin.
- **Upload succeeds but send fails with "no active integration"** — the WhatsApp integration was disconnected. Reconnect via [Integrations](#/integrations/connect-whatsapp).
- **The wrong file-type icon shows on the recipient's phone** — re-pick the file and send again. WhatsApp sometimes infers the wrong type from the extension.

## General

- **A send that used to work suddenly fails** — the integration's access token likely expired. Go to Settings → Integrations, open the integration, and click **Test connection**.
- **You see two copies of the same message in the thread** — refresh once. If it keeps happening, report it.

## Related

- [Inbox overview](#/inbox/overview) — three-pane layout and real-time updates.
- [Provider calls log](#/audit-telemetry/provider-calls) — every Meta invocation, with request and response (admin-only).
- [Integrations](#/integrations/overview) — reconnect or rotate credentials.
