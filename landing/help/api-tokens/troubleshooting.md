# Troubleshooting API tokens

## `401 Unauthorized` with an apparently valid token

Work through these in order:

1. **Check the format.** The full plaintext is `nk_<8-char-prefix>_<40-char-secret>` — three underscore-separated segments. If your copy is missing the trailing secret (common when you copy from a truncated terminal), it will always 401.
2. **Check for surrounding whitespace.** A trailing newline in an env var (`NUDGEWAY_API_TOKEN=$(cat file)`) is a frequent culprit. Prefer `read -r` or paste inline.
3. **Check the header shape.** `Authorization: Bearer <token>`. `Bearer ` (with the space) is required.
4. **Check the token isn't revoked or expired.** Open `/settings/api-tokens`, look for the prefix. A `Revoked` badge or a past `expires_at` explains the 401.
5. **Check the minting user is still active.** Revoking the user account revokes all its tokens.

## Usage log rows never appear for my token

- **Are you actually using bearer auth?** The usage log is populated **only** for `Authorization: Bearer` requests. Session-cookie calls are not recorded here — check the audit log instead.
- **MCP forwarder auth precedence.** If both `NUDGEWAY_API_TOKEN` and `NUDGEWAY_SESSION_COOKIE` are set, the forwarder uses the token and drops the cookie. If only the cookie is set, requests go through session auth and never hit the token usage middleware. Confirm your MCP stanza sets `NUDGEWAY_API_TOKEN`.
- **Detached-goroutine backlog.** Writes happen after the response flushes. Under load or during a MySQL blip a small delay is expected. If rows never arrive, check server logs for `api_token_usage insert failed` warnings.
- **Wrong token.** Multiple tokens with similar names — verify the prefix in the drawer matches the prefix your client is presenting.

## CSRF still required by mistake

Symptom: a bearer request returns `403 CSRF token missing`.

- **The middleware order is wrong.** Bearer authentication must run before the CSRF middleware for the double-submit check to be skipped. If you have a custom deployment with reordered middleware, restore the standard order (see `internal/infrastructure/http/middleware/`).
- **You accidentally sent both auth methods.** If the request includes both `Authorization: Bearer <token>` **and** `Cookie: nudgeway_session=<...>`, the server takes the cookie path and enforces CSRF. Strip the session cookie from bearer-authenticated requests. The MCP forwarder does this automatically when `NUDGEWAY_API_TOKEN` is set.
- **The token was rejected and fell through to the cookie path.** Check for a prior `401` on the token itself — a rejected token means the request is treated as unauthenticated, and any accompanying session cookie is then honoured.

## `403 Forbidden` on create / revoke

The calling user lacks the `api_tokens.manage` RBAC scope. Org admins have it by default; other roles do not.

## `403 Forbidden` on usage / metrics

Users can read **their own** token's usage. Org admins can read any token in the org. Reading another user's token's usage from a non-admin session returns `403`.

## Related

- [Overview](/#/api-tokens/overview)
- [Create a token](/#/api-tokens/create-token)
- [Revoke a token](/#/api-tokens/revoke-token)
- [Usage log + metrics](/#/api-tokens/usage-log-metrics)
- [MCP server](/#/developer/mcp-server)
