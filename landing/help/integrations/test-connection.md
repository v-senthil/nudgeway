# Test the connection

**Test connection** asks Meta whether the credentials you saved on the integration are still working, and updates the status pill accordingly. Use it right after creating an integration, right after rotating any of the six fields, or any time you suspect something is wrong.

## How to use

1. Click **Settings** -> **Integrations**.
2. Find the integration row and click **Test connection**.
3. Watch the status pill. A green check with `connected` means the credentials work and Meta is reachable. A red banner with a Meta error means something needs fixing.

## What the status pill means afterward

- **Connected** — everything is fine.
- **Degraded** — Meta returned an error (timeout, 5xx, temporary throttle). The workspace may still try, but re-test in a few minutes.
- **Auth failed** — Meta rejected the access token. See below.
- **Rate limited** — Meta is throttling this WABA. Wait, then re-test.

## Troubleshooting

- **Status pill flips to "auth failed"** — the access token has expired or been revoked. Generate a new permanent System User token in Meta (see [Connect a WhatsApp integration](#/integrations/connect-whatsapp) for the exact path), delete the integration in Nudgeway, and re-create it with the new token. There's no in-place rotate in the UI today.
- **Status pill flips to "degraded" with a "bad phone_number_id" error** — you pasted the wrong Phone Number ID when creating the integration. Open the Meta App Dashboard -> WhatsApp -> API Setup, copy the numeric id under "From", delete and re-create the integration.
- **Status pill flips to "rate limited"** — Meta is throttling. Wait 5-15 minutes and click **Test connection** again. Repeated rate limiting means you need to talk to your Meta representative.
- **Test connection button spins and never resolves** — the workspace can't reach Meta from where it's running. If you're testing against a local install, check your network. Otherwise contact your admin.

## Related

- [Integrations overview](#/integrations/overview)
- [Connect a WhatsApp integration](#/integrations/connect-whatsapp)
- [Meta API execution log](#/audit-telemetry/provider-calls)
