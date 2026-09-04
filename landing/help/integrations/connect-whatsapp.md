# Connect a WhatsApp integration

Adding a WhatsApp integration takes six fields, all of which come from your Meta App Dashboard. Nudgeway encrypts the secret fields before storing them and never shows them back.

## The six fields

| Field | Where to find it in Meta |
|---|---|
| **Name** | Any label you like — this is what shows in the Nudgeway integration list. Example: "Acme India". |
| **Phone Number ID** | developer.facebook.com -> your App -> **WhatsApp -> API Setup** -> the number under "From". Copy the numeric id, not the display number. |
| **WABA ID** | The same **API Setup** page, at the top under "WhatsApp Business Account ID". |
| **Access Token** | Meta Business Settings -> **Users -> System Users** -> your system user -> **Generate new token**. Grant the scopes `whatsapp_business_messaging` and `whatsapp_business_management`. Use a permanent System User token, not the 24-hour test token — the test token will break your integration when it expires. |
| **App Secret** | developer.facebook.com -> your App -> **Settings -> Basic** -> **App Secret -> Show**. Nudgeway uses this to verify that webhooks really came from Meta. |
| **Verify Token** | Any random string you choose. You'll paste the same value into Meta's webhook Callback URL panel later. It's how Meta and Nudgeway confirm the webhook subscription. A password-generator string of ~32 characters is fine. |

## How to use

1. Copy the six values above out of Meta into a scratch pad.
2. In Nudgeway, click **Settings** -> **Integrations** -> **Connect WhatsApp**.
3. Paste the six values into the form.
4. Click **Save**.

After saving, you'll see the new integration in the list with a Webhook URL displayed on its detail panel — you'll paste that URL into Meta as the next step. Then:

1. Click **Test connection** on the new row to confirm the credentials work. See [Test the connection](#/integrations/test-connection).
2. Click **Push to Meta** (or paste the Webhook URL into Meta manually). See [Push webhook to Meta](#/integrations/webhook-setup).

## Gotchas

- **Never paste secrets into the Name or ID fields**. The form validates and rejects that, but double-check that access token, app secret, and verify token each go into their own labelled field.
- **Two integrations cannot share the same Phone Number ID**. If you're moving a number between workspaces, delete the old integration first.
- **Temporary access tokens expire in 24 hours**. Use a permanent System User token.

## Related

- [Integrations overview](#/integrations/overview)
- [Test the connection](#/integrations/test-connection)
- [Push webhook to Meta](#/integrations/webhook-setup)
- [First run](#/getting-started/first-run)
