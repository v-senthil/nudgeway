---
name: nudgeway-api-tokens
description: Nudgeway API tokens — mint, list, and revoke opaque bearer credentials used by the MCP server, CLI clients, and third-party integrations. Tokens are `argon2id`-hashed at rest; the plaintext is returned exactly once.
trigger: User asks about API keys, API tokens, bearer credentials, personal access tokens, wiring the MCP server auth, or replacing session-cookie auth for scripted API access.
---

# Nudgeway API tokens skill

## Overview

An **API token** is an opaque bearer credential scoped to the minting user's org and RBAC permissions. It is the recommended auth path for the MCP server (`./bin/nudgeway-mcp`), the CLI, and any external script that needs to hit `/api/v1/*` without a browser session.

Plaintext shape: `nk_<8-char-prefix>_<40-char-secret>` (base32).
- The **prefix** is stored in cleartext and is used as the lookup key.
- The **secret** is `argon2id`-hashed at rest — never persisted or logged in plaintext.
- The full plaintext token is returned **exactly once**, in the create response. If lost, revoke and mint a new one.

Bearer path skips CSRF: when the request carries `Authorization: Bearer <token>` the backend middleware does not require the `X-CSRF-Token` double-submit.

## MCP tools

| operationId | Purpose |
|---|---|
| `createAPIToken` | Mint a new token. Body: `{name, expires_at?}`. Response includes the plaintext `token` once. |
| `listAPITokens` | List tokens visible to the caller (metadata only — no plaintext). |
| `revokeAPIToken` | Revoke a token by ID. Idempotent. |

## REST equivalents

```
POST   /api/v1/api-tokens
GET    /api/v1/api-tokens
DELETE /api/v1/api-tokens/{id}
```

All three are guarded by the `api_tokens.manage` RBAC scope.

## Patterns

### Mint a token for a new MCP client

```json
{
  "tool": "createAPIToken",
  "arguments": {
    "body": {
      "name": "claude-desktop-laptop",
      "expires_at": "2027-01-01T00:00:00Z"
    }
  }
}
```

Response (once, then never again):

```json
{
  "id": "01JC5...",
  "name": "claude-desktop-laptop",
  "prefix": "nk_abcd1234",
  "token": "nk_abcd1234_<40-char-secret>",
  "created_at": "…",
  "expires_at": "2027-01-01T00:00:00Z"
}
```

Paste `token` into the MCP client's `NUDGEWAY_API_TOKEN` env var.

### Rotate

1. `createAPIToken` with a new name (e.g. append `-2`).
2. Update the client env / config with the new plaintext.
3. `revokeAPIToken` on the old token's `id`.

### Audit

`listAPITokens` returns metadata (`id`, `name`, `prefix`, `created_at`, `last_used_at`, `expires_at`, `revoked_at`) — pair with `getAuditLogs` to trace usage over time.

## Gotchas

- **Plaintext is shown once.** No recovery flow. Lost token → revoke + remint.
- **Bearer skips CSRF.** State-changing calls succeed without an `X-CSRF-Token` header when authenticated via bearer. The session-cookie path still enforces CSRF.
- **RBAC inherits from the minting user.** A token can never do more than its owner. Revoking the owner's user account revokes the tokens.
- **Prefix is public-ish.** It appears in `listAPITokens` and audit logs — treat it as an identifier, not a secret. The secret half is what matters.
- **No wildcard token.** There is no org-wide or admin-bypass token; every token is bound to a user.

## Related skills

- [`../nudgeway-mcp/SKILL.md`](../nudgeway-mcp/SKILL.md) — how the MCP forwarder consumes `NUDGEWAY_API_TOKEN`.
- [`../nudgeway-integrations/SKILL.md`](../nudgeway-integrations/SKILL.md) — provider credentials (Meta access tokens etc.) are separate from Nudgeway API tokens; do not confuse the two.
