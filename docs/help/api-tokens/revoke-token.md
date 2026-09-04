# Revoke a token

Revoke a token the moment you suspect it's been leaked, when a teammate leaves, or when you're retiring the client that was using it. Revocation takes effect immediately — the next request the client makes will be rejected.

## How to use

1. Go to **Settings → API tokens**.
2. Find the row by name or by the prefix you copied earlier.
3. Click the row's overflow menu and choose **Revoke**.
4. Confirm.

The row stays in the list with a **Revoked** chip and the time it was revoked, so you can still see what the token was doing beforehand. A revoked token cannot be reactivated — if you need access again, mint a new one.

## Rotation without downtime

To swap a token without cutting a client off:

1. Click **New token** and give it a name that makes the pairing obvious (for example, append `-v2`).
2. Copy the new plaintext into the client's configuration and restart it.
3. Open the tokens list, click the new row, and confirm the **Last used** timestamp starts advancing — that means the client is authenticating on the new token.
4. Once you're confident, revoke the old row.

## Troubleshooting

- **Lost the token.** Click **Revoke** on that row and mint a fresh one. There's no way to re-display a plaintext you didn't copy.
- **The client is still working after I revoked.** The client may still be reading a cached value. Restart it, then confirm the old row's **Last used** timestamp has stopped advancing and the new row's is moving.
- **I can't find the Revoke option.** You need the API-tokens permission. Ask an organisation admin to revoke it for you.
- **I revoked the wrong one.** Revocation is not reversible. Mint a replacement immediately and update the client that was using the revoked token.

## Related

- [Overview](#/api-tokens/overview)
- [Create a token](#/api-tokens/create-token)
- [Usage log and metrics](#/api-tokens/usage-log-metrics)
- [Troubleshooting](#/api-tokens/troubleshooting)
