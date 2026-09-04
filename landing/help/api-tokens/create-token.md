# Create a token

Mint a token when an MCP client, vendor integration, or automation script needs to talk to Nudgeway on your behalf. The plaintext value is shown **once** — copy it before you close the dialog.

## How to use

1. Sign into Nudgeway.
2. Go to **Settings → API tokens**.
3. Click **New token**.
4. Enter a **name** that identifies where the token will live — for example `Prod MCP`, `Claude Desktop — laptop`, or `CI`.
5. Pick an **expiry** in days, or leave it blank for a non-expiring token. If you're unsure, 90 days is a good default; you can always mint a fresh one.
6. Click **Create**.
7. A dialog shows the full token in the shape `nk_<prefix>_<secret>`. Click the copy icon and paste it straight into the client that needs it, or into your password manager.
8. Click **Done**. The plaintext will not appear again.

The new token appears in the list with its prefix, name, creation time, and (if you set one) expiry. The **Last used** column stays blank until the receiving client makes its first request, then updates automatically.

## Handing the token to whoever needs it

Most MCP clients and vendor tools have a field labelled something like "API token" or "Bearer token" — paste the full `nk_..._...` value into that field. Behind the scenes the client will send it in the request's `Authorization` header; you don't need to format anything yourself.

If you're passing the token to a vendor or a teammate, share it over a secure channel (password manager, secrets vault) — never in plain email or chat.

## Troubleshooting

- **I closed the dialog before copying the plaintext.** There is no way to recover it. Revoke the token and create a new one — see [Revoke a token](#/api-tokens/revoke-token).
- **The Create button is disabled or greyed out.** You need the API-tokens permission on your account. Ask an organisation admin to mint the token for you, or to grant you the permission.
- **The name field rejects my input.** Names must be non-empty. Pick something short and descriptive.

## Related

- [Overview](#/api-tokens/overview)
- [Revoke a token](#/api-tokens/revoke-token)
- [Usage log and metrics](#/api-tokens/usage-log-metrics)
- [Troubleshooting](#/api-tokens/troubleshooting)
