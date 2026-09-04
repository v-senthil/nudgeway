# Push webhook to Meta

Meta needs a publicly-reachable HTTPS URL to send WhatsApp events to. Each integration row exposes its own Webhook URL — for example, `https://app.example.com/webhooks/whatsapp/<integration-id>`. There are two ways to get that URL registered with Meta.

## Option A — Push to Meta button (recommended in dev)

If you're running Nudgeway locally with an ngrok tunnel, use the built-in button:

1. Click **Settings** -> **Integrations** -> the integration row.
2. Open the **Details** tab.
3. Click **Push to Meta**. Nudgeway detects your local ngrok tunnel automatically, combines it with the integration id, and registers the resulting URL with Meta. You never type your verify token again — it's reused from what you saved when creating the integration.
4. A success toast confirms the URL that was registered.

If Nudgeway can't detect a tunnel, you'll see a message asking you to paste one manually. Start `ngrok http 8080` and try again, or use Option B.

## Option B — Paste into Meta manually

If you're not using ngrok, or you'd rather do it in the Meta console:

1. Copy the **Webhook URL** shown on the integration's Details tab.
2. Open developer.facebook.com -> your App -> **WhatsApp -> Configuration -> Callback URL**.
3. Paste the URL.
4. Paste the same **Verify Token** you used when creating the integration in Nudgeway.
5. Click **Verify and save**. Meta will hit Nudgeway to confirm the handshake; a green tick means it worked.
6. Under **Webhook fields**, subscribe to at least: `messages`, `message_status`, `calls`, `call_settings_update`.

## Production note

Production installs verify every webhook using the App Secret you saved. If the App Secret in Nudgeway drifts from the one in the Meta App Dashboard, webhooks stop being accepted. Delete and re-create the integration with the current App Secret if this happens.

## Troubleshooting

- **Meta shows a red "Verify and save" error** — the Verify Token pasted into Meta doesn't match the one you saved in Nudgeway. Delete the integration and re-create it with a fresh verify token, then paste that value into both sides.
- **"Push to Meta" says "tunnel not detected"** — ngrok isn't running, or it's on a non-default port. Start `ngrok http 8080` and click **Push to Meta** again. Or use Option B.
- **"Push to Meta" says "invalid URL — HTTPS required"** — Meta only accepts `https://` webhook URLs. ngrok and Cloudflare Tunnel both give you HTTPS by default; a plain `http://localhost` won't work.
- **Meta accepts the URL but webhooks never arrive** — confirm you subscribed to the webhook fields (Option B step 6). Meta's Callback URL panel has a "Test" button that will fire a sample event.
- **Deliveries land but are rejected** — the App Secret in Nudgeway no longer matches Meta's. Delete the integration and re-create it with the current App Secret.

## Related

- [Integrations overview](#/integrations/overview)
- [Connect a WhatsApp integration](#/integrations/connect-whatsapp)
- [First run](#/getting-started/first-run)
