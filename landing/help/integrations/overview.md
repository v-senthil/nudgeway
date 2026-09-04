# Integrations

An **Integration** is one connection to a provider. For WhatsApp today, one integration = **one Meta Phone Number ID**. If you run three phone numbers across two WABAs, you have three integrations.

Non-secret config (Phone Number ID, WABA ID) is stored alongside the integration row. Secrets — access token, app secret, verify token — are encrypted with a workspace key and are never shown back once you've saved them.

## What ships today

| Provider | Type | Notes |
|---|---|---|
| WhatsApp | Channel | Meta WhatsApp Business Cloud API |

Other providers (Zoho Desk, OpenAI, ...) are on the roadmap.

## Status pill

Each integration row shows a status pill:

| Pill | Meaning |
|---|---|
| **Connected** | Health check green. The workspace is actively using this integration. |
| **Disconnected** | Soft-disconnected. Sends and webhooks are paused for this integration, but the row is preserved. |
| **Degraded** | The last health check failed. The workspace may still try — click **Test connection** to re-check. |
| **Auth failed** | Meta rejected the access token. Update the token. |
| **Rate limited** | Meta is throttling this account. Wait, then re-test. |

## Multi-tenancy

Every integration is scoped to your organization. Nobody outside your workspace can read, edit, or receive webhooks against your integrations.

## Pages

- [Connect a WhatsApp integration](#/integrations/connect-whatsapp)
- [Test the connection](#/integrations/test-connection)
- [Push webhook to Meta](#/integrations/webhook-setup)
- [Business profile](#/integrations/business-profile)
- [Call settings](#/integrations/call-settings)
- [Official Business Account (OBA)](#/integrations/oba-status)
- [Business username](#/integrations/business-username)
- [Phone number details + QR](#/integrations/phone-number-details)
- [Delete an integration](#/integrations/delete-integration)
