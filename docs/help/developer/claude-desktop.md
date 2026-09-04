# Claude Desktop setup

Wire Claude Desktop to the Nudgeway MCP server so the model can call every REST endpoint on your behalf.

## Prereqs

- `./bin/nudgeway-mcp` built (`make mcp`).
- The Nudgeway server running on `http://127.0.0.1:8080` (`make dev`).
- An API token minted via `/settings/api-tokens` — see [Create a token](/#/api-tokens/create-token).

## Config file

macOS:

```
~/Library/Application Support/Claude/claude_desktop_config.json
```

Windows and Linux paths follow the platform Claude Desktop docs.

## Full JSON example — token auth (preferred)

```json
{
  "mcpServers": {
    "nudgeway": {
      "command": "/absolute/path/to/nudgeway/bin/nudgeway-mcp",
      "env": {
        "NUDGEWAY_API_URL": "http://127.0.0.1:8080",
        "NUDGEWAY_API_TOKEN": "nk_abcd1234_efghijklmnopqrstuvwxyz234567abcdefghijklmnop"
      }
    }
  }
}
```

Restart Claude Desktop. The Nudgeway toolset appears under the tools menu — one tool per OpenAPI operation.

## Session-cookie fallback

Use this only if you don't yet have a token minted and want to poke the API from a fresh browser login.

1. Log into Nudgeway in your browser (`http://localhost:5173`).
2. Open DevTools → **Application** → **Cookies** → `http://localhost:5173` (or `http://localhost:8080` if you loaded the API origin directly).
3. Copy the `nudgeway_session` cookie value into `NUDGEWAY_SESSION_COOKIE`.
4. Copy the `nudgeway_csrf` cookie value (or fetch it via `GET /api/v1/auth/csrf`) into `NUDGEWAY_CSRF_TOKEN`. This is required for `POST` / `PUT` / `PATCH` / `DELETE` tool calls.

```json
{
  "mcpServers": {
    "nudgeway": {
      "command": "/absolute/path/to/nudgeway/bin/nudgeway-mcp",
      "env": {
        "NUDGEWAY_API_URL": "http://127.0.0.1:8080",
        "NUDGEWAY_SESSION_COOKIE": "<paste nudgeway_session value>",
        "NUDGEWAY_CSRF_TOKEN": "<paste nudgeway_csrf value>"
      }
    }
  }
}
```

Cookies are 30-day sliding; refresh when they expire.

If both `NUDGEWAY_API_TOKEN` and `NUDGEWAY_SESSION_COOKIE` are set, the forwarder uses the token and drops the cookie values entirely.

## First calls to try

Ask Claude to run these tools:

- `listIntegrations` — see your connected WhatsApp integrations.
- `listConversations` — the inbox thread list.
- `listAPITokens` — verify the token you're authenticating with appears.
- `postMessagesSend` — send a WhatsApp message. See [Send a text message](/#/inbox/send-text).

## Troubleshooting

- **Toolset does not appear.** Fully quit and relaunch Claude Desktop. Config is read at startup only.
- **`command not found` on launch.** Use an absolute path to `nudgeway-mcp`. Claude Desktop does not resolve `PATH` the way your shell does.
- **All tool calls return `connection refused`.** The Nudgeway server is not running on `NUDGEWAY_API_URL`. Start it with `make dev`.
- **`401` on every tool.** Token is missing, malformed, revoked, or expired. Walk the checklist in [API tokens troubleshooting](/#/api-tokens/troubleshooting).
- **`403 CSRF token missing` on cookie mode.** `NUDGEWAY_CSRF_TOKEN` is empty or stale — copy a fresh value from the browser cookie jar.
- **Tool list is much smaller than expected.** Your `nudgeway-mcp` binary is out of date. Rebuild with `make mcp`.

## Related

- [MCP server](/#/developer/mcp-server) — build, protocol, auth precedence.
- [Create a token](/#/api-tokens/create-token) — mint bearer credentials.
- [Skills library](/#/developer/skills) — playbooks that pair with MCP tools.
