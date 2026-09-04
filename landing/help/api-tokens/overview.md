# API tokens

An **API token** is a credential you mint in Nudgeway and hand to an outside client — most often an MCP client (Claude Desktop, an agent framework, a vendor's automation) so it can act on your organisation's behalf without a browser session.

You create tokens under **Settings → API tokens**. Each token is bound to the user who minted it and inherits that user's permissions. A token can never do more than its owner can.

## Format

Every token looks like `nk_<8-char-prefix>_<40-char-secret>`.

- The **prefix** is what you'll see in the tokens list and in usage rows. Treat it as an identifier — it's fine to share when someone needs to help you find the right row.
- The **secret** is the sensitive part. Nudgeway shows the full plaintext value exactly once, at the moment of creation, and never again. If you lose it, revoke the token and mint a new one.

## What a token can do

- It carries the same permissions as the user who created it.
- Deactivating or removing that user revokes all their tokens automatically.
- There is no organisation-wide or admin-bypass token. Every token is tied to one person.

## Where you'll use them

Hand the plaintext value to whoever needs programmatic access. In practice that means pasting it into an MCP client's configuration, a vendor's integration setup, or a script that runs on your behalf. The receiving system sends it back to Nudgeway as an `Authorization` header — you don't need to do anything special beyond copying the value across.

## Related

- [Create a token](#/api-tokens/create-token)
- [Revoke a token](#/api-tokens/revoke-token)
- [Usage log and metrics](#/api-tokens/usage-log-metrics)
- [Troubleshooting](#/api-tokens/troubleshooting)
