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
        "NUDGEWAY_SESSION_COOKIE": "<value of nudgeway_session cookie>",
        "NUDGEWAY_CSRF_TOKEN": "<value of nudgeway_csrf cookie>"
      }
    }
  }
}
```

Restart Claude Desktop. All 23 (and growing) Nudgeway tools appear in the MCP menu.

## Extracting the session cookie

1. Log into Nudgeway in your browser (http://localhost:5173).
2. DevTools → Application → Cookies → `http://localhost:5173`.
3. Copy the `nudgeway_session` value into `NUDGEWAY_SESSION_COOKIE`.
4. Copy the `nudgeway_csrf` value into `NUDGEWAY_CSRF_TOKEN`.

Cookies are 30-day sliding; refresh the values when they expire.

## Tool naming

Tools match OpenAPI `operationId` verbatim. E.g. `listIntegrations`, `postMessagesSend`, `testIntegration`. If you can't find a tool, run `./bin/nudgeway-mcp --list-tools | jq '.[].name'` to enumerate.

## MCP methods implemented

| Method | Behaviour |
|---|---|
| `initialize` | Reports server info + `tools` capability. |
| `ping` | Empty result. |
| `tools/list` | Returns every tool descriptor generated from openapi.yaml. |
| `tools/call` | Forwards to the REST API; success = tool response body as text; failure = `isError: true` with the error body inline. |
| `notifications/*` | Silently accepted (per MCP spec, notifications have no reply). |

## Gotchas

- **Auth is session-cookie based**. There is no API-key surface yet. Any credential rotation requires re-logging-in and re-extracting the cookies.
- **CSRF is double-submit**. The forwarder sets both `X-CSRF-Token` header AND the `nudgeway_csrf` cookie on state-changing methods — matching the backend's expectation (`internal/infrastructure/auth/csrf.go`).
- **The MCP server does not embed the REST server**. It forwards over HTTP to a running Nudgeway process; if the server is down the tools return connection errors.
- **New endpoints auto-appear** after a `make mcp` rebuild — no manual tool registration. See CLAUDE.md §8 for the OpenAPI → REST → MCP change flow.

## Related skills

- Every other skill in this directory maps to a subset of MCP tools; pair this skill with the domain skill you need.
- [`../README.md`](../README.md) — index of all skills.
