---
name: nudgeway-mcp
description: Bring up the Nudgeway MCP server and wire it into an MCP client (Claude Desktop, Claude Code, Cursor, any). Every OpenAPI operation becomes a tool named after its operationId.
trigger: User asks about MCP, connecting Claude Desktop to Nudgeway, exposing the API to an agent, or programmatic access to the inbox / integrations.
---

# Nudgeway MCP skill

## Overview

`cmd/mcp` is a standalone Go binary that reads `internal/api/openapi/openapi.yaml` at boot, generates one MCP tool per operation (auto-derived JSON Schema from path/query/body params), and forwards tool calls to the running Nudgeway REST API. It speaks MCP protocol version `2024-11-05` over stdio.

## Build + inspect

```bash
make mcp                    # → ./bin/nudgeway-mcp (~6.6 MB)
./bin/nudgeway-mcp --list-tools    # dump the tool descriptors as JSON
```

## Wire into Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "nudgeway": {
      "command": "/absolute/path/to/nudgeway/bin/nudgeway-mcp",
      "env": {
        "NUDGEWAY_API_URL": "http://127.0.0.1:8080",
        "NUDGEWAY_API_TOKEN": "nk_abcd1234_<40-char-secret>"
      }
    }
  }
}
```

Restart Claude Desktop. All Nudgeway tools appear in the MCP menu. On startup the server logs the active auth mode to stderr, e.g. `nudgeway-mcp: base=http://127.0.0.1:8080 auth=api-token tools=23`.

## Authentication

Two modes; **prefer API tokens** unless you're doing throwaway local poking.

### API tokens (recommended)

Plaintext shape: `nk_<8-char-prefix>_<40-char-secret>` (base32). Sent as `Authorization: Bearer <token>`. No CSRF header required — the backend's bearer middleware skips the double-submit check.

Mint one:

- **UI**: `/settings/api-tokens` → *New token*. Copy the plaintext once; it's `argon2id`-hashed at rest and won't be shown again.
- **MCP tool**: `createAPIToken` with body `{"name": "…", "expires_at": "…"}` — the response returns the plaintext `token` field one time. Companion tools: `listAPITokens`, `revokeAPIToken`.

Then:

```bash
export NUDGEWAY_API_TOKEN=nk_abcd1234_<40-char-secret>
./bin/nudgeway-mcp
```

### Session cookie (fallback for local dev)

Only use when you don't have a token minted yet.

1. Log into Nudgeway in your browser (http://localhost:5173).
2. DevTools → Application → Cookies → `http://localhost:5173`.
3. Copy the `nudgeway_session` value into `NUDGEWAY_SESSION_COOKIE`.
4. Copy the `nudgeway_csrf` value into `NUDGEWAY_CSRF_TOKEN` (required for POST/PUT/PATCH/DELETE).

Cookies are 30-day sliding; refresh when they expire. If `NUDGEWAY_API_TOKEN` is also set, cookie values are ignored.

## Tool naming

Tools match OpenAPI `operationId` verbatim. E.g. `listIntegrations`, `postMessagesSend`, `testIntegration`, `createAPIToken`, `listAPITokens`, `revokeAPIToken`. If you can't find a tool, run `./bin/nudgeway-mcp --list-tools | jq '.[].name'` to enumerate.

## MCP methods implemented

| Method | Behaviour |
|---|---|
| `initialize` | Reports server info + `tools` capability. |
| `ping` | Empty result. |
| `tools/list` | Returns every tool descriptor generated from openapi.yaml. |
| `tools/call` | Forwards to the REST API; success = tool response body as text; failure = `isError: true` with the error body inline. |
| `notifications/*` | Silently accepted (per MCP spec, notifications have no reply). |

## Gotchas

- **API token wins over session cookie.** If both `NUDGEWAY_API_TOKEN` and `NUDGEWAY_SESSION_COOKIE` are set, the forwarder uses the token and drops the cookie entirely. Also drops CSRF — the bearer middleware doesn't require it.
- **Plaintext token is shown once.** Lost tokens are unrecoverable; revoke and mint a new one via `/settings/api-tokens` or the `createAPIToken` MCP tool.
- **CSRF still applies to the cookie fallback.** When you're on `NUDGEWAY_SESSION_COOKIE`, the forwarder sets both `X-CSRF-Token` and the `nudgeway_csrf` cookie on state-changing methods — matching the backend's expectation (`internal/infrastructure/auth/csrf.go`).
- **The MCP server does not embed the REST server**. It forwards over HTTP to a running Nudgeway process; if the server is down the tools return connection errors.
- **New endpoints auto-appear** after a `make mcp` rebuild — no manual tool registration. See CLAUDE.md §8 for the OpenAPI → REST → MCP change flow.

## Related skills

- Every other skill in this directory maps to a subset of MCP tools; pair this skill with the domain skill you need.
- [`../README.md`](../README.md) — index of all skills.
