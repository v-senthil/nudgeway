# Create a token

Mint a new API token from the settings UI or, if you already have a working MCP client, from the `createAPIToken` tool. The plaintext value is returned **exactly once** — copy it into your client immediately.

## How to use

1. Sign into Nudgeway.
2. Open **Settings → API tokens** (`/settings/api-tokens`).
3. Click **New token**.
4. Pick a name (e.g. `claude-desktop-laptop`, `gh-actions-ci`) and an optional expiry in days.
5. Click **Create**.
6. A modal displays the full plaintext token in the shape `nk_<8-char-prefix>_<40-char-secret>`. Copy it into a password manager or your MCP client config now.
7. Click **Done**. The plaintext will not be shown again.

The new token appears in the list with its prefix, name, creation timestamp, and (if set) expiry. `last_used_at` populates asynchronously the first time the token authenticates a request.

## API

```
POST /api/v1/api-tokens
```

Request body:

```json
{
  "name": "claude-desktop-laptop",
  "expires_in_days": 365
}
```

`expires_in_days` is optional — omit for a non-expiring token.

Response (200):

```json
{
  "id": "01JC5XYZTOKENID",
  "name": "claude-desktop-laptop",
  "prefix": "abcd1234",
  "plaintext": "nk_abcd1234_efghijklmnopqrstuvwxyz234567abcdefghijklmnop",
  "created_at": "2026-09-05T10:15:00Z",
  "expires_at": "2027-09-05T10:15:00Z"
}
```

The `plaintext` field is the FULL token and is returned exactly once. It is never persisted server-side; only the prefix and an `argon2id` hash of the secret are stored.

Guarded by the `api_tokens.manage` RBAC scope.

## MCP

```json
{
  "tool": "createAPIToken",
  "arguments": {
    "body": {
      "name": "claude-desktop-laptop",
      "expires_in_days": 365
    }
  }
}
```

The response includes the plaintext `token` / `plaintext` field once. Paste it into the calling client's `NUDGEWAY_API_TOKEN` env var.

Common pattern: use an existing session-cookie MCP setup to mint a proper token, then rotate the client onto bearer auth.

## Troubleshooting

- **`400 name: required`** — the request body was missing `name`, or `name` was empty.
- **`403`** — the calling user lacks the `api_tokens.manage` scope.
- **I closed the modal before copying the plaintext.** There's no recovery flow. Revoke the token via [Revoke a token](/#/api-tokens/revoke-token) and create a new one.

## Related

- [Overview](/#/api-tokens/overview)
- [Revoke a token](/#/api-tokens/revoke-token)
- [Usage log + metrics](/#/api-tokens/usage-log-metrics)
- [MCP server](/#/developer/mcp-server)
