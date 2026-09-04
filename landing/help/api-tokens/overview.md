# API tokens

An **API token** is an opaque bearer credential scoped to the minting user's org and RBAC permissions. Tokens are the recommended auth path for the MCP server, the CLI, and any external script that needs to hit `/api/v1/*` without a browser session.

## Format

Plaintext shape: `nk_<8-char-prefix>_<40-char-secret>` (base32).

- The **prefix** is stored in cleartext and used as the lookup key. It appears in `listAPITokens` responses and audit logs — treat it as an identifier, not a secret.
- The **secret** is `argon2id`-hashed at rest. It is never persisted or logged in plaintext.
- The full plaintext token is returned **exactly once**, in the response to `createAPIToken`. If lost, revoke and mint a new one — there is no recovery flow.

## Bearer path skips CSRF

When a request carries `Authorization: Bearer <token>`, the backend's bearer middleware short-circuits the CSRF double-submit check. This is intentional and matches the standard behaviour for token-authenticated APIs — CSRF is a browser-form-submission defence, and bearer tokens are not sent automatically by browsers.

The session-cookie path continues to enforce CSRF on every `POST` / `PUT` / `PATCH` / `DELETE`.

## What a token can do

- Same RBAC scopes as the user that minted it. A token can never do more than its owner.
- Revoking the owner's user account revokes all their tokens.
- There is no wildcard, org-wide, or admin-bypass token. Every token is bound to a single user.

## Where they're used

- **MCP server** — `NUDGEWAY_API_TOKEN` env var on `./bin/nudgeway-mcp`. See [MCP server](/#/developer/mcp-server).
- **CI + scripts** — `Authorization: Bearer <token>` on any `/api/v1/*` request.
- **CLI clients** — any script that needs long-lived, revocable access.

## Related

- [Create a token](/#/api-tokens/create-token)
- [Revoke a token](/#/api-tokens/revoke-token)
- [Usage log + metrics](/#/api-tokens/usage-log-metrics)
- [MCP server](/#/developer/mcp-server)
- [REST API](/#/developer/rest-api)
