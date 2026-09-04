# Troubleshooting API tokens

Common issues you'll see when handing a token to an MCP client, vendor, or automation, and how to resolve them from the UI.

## The vendor says the token doesn't work

Open **Settings → API tokens** and find the row by its prefix (the `nk_<prefix>_...` portion is safe to ask for).

- If the row shows a **Revoked** chip, mint a new token, hand it over, and ask the vendor to update their configuration.
- If the row shows an expiry date in the past, the token has expired. Create a fresh one with a longer expiry.
- If the row looks healthy but the **Last used** column isn't updating after the vendor retries, the client is probably sending a different value than the one you handed them — copy the token again from your password manager and re-send it. If you didn't save it, revoke and mint a new one.
- If the prefix the vendor is showing you doesn't match any row in the list, they're on the wrong token entirely. Send them the correct one.

## Lost the token

There is no way to redisplay a plaintext value. Click **Revoke** on that row and create a fresh one — see [Create a token](#/api-tokens/create-token). Save the new value in a password manager before you close the creation dialog.

## Suspect the token has been leaked

Revoke it immediately (see [Revoke a token](#/api-tokens/revoke-token)). Then open the token's drawer and check the **Log** tab for unfamiliar source IPs or unexpected activity in the hours before you revoked. Mint a replacement, hand it to the legitimate client through a secure channel, and update wherever the old value was stored.

## The Create or Revoke button is missing

You don't have the API-tokens permission on your account. Ask an organisation admin to mint or revoke the token for you, or to grant you the permission.

## A teammate can see tokens they shouldn't

Non-admin users can only see and act on their own tokens. Organisation admins can see every token in the org. If someone reports seeing tokens they shouldn't, check their role in **Settings → Team**.

## I can't tell which token a client is using

Ask the client's operator (or the vendor) to read out the prefix — the `nk_<prefix>_...` portion is not sensitive. Match the prefix against the rows under **Settings → API tokens**. If two tokens have similar names, the prefix is the unambiguous identifier.

## The token was working, now it isn't

Check the row for the user who minted the token. If that user was deactivated or removed from the organisation, all their tokens are revoked automatically. Mint a new token from an active user account and hand it over.

## Related

- [Overview](#/api-tokens/overview)
- [Create a token](#/api-tokens/create-token)
- [Revoke a token](#/api-tokens/revoke-token)
- [Usage log and metrics](#/api-tokens/usage-log-metrics)
