# MCP server

Nudgeway ships a standalone Model Context Protocol (MCP) server —
`cmd/mcp` — that exposes **every** operation defined in
`internal/api/openapi/openapi.yaml` as an MCP tool. Any MCP-capable
client (Claude Desktop, Claude Code, Cursor, custom SDKs) can drive the
Nudgeway REST API in natural language once the server is wired up.

The spec is embedded at build time via `//go:embed openapi.yaml`, so the
binary is fully self-contained — no config, no external YAML at runtime.

## What it exposes

- One MCP tool per OpenAPI `operationId`. Tool name equals the
  operationId verbatim (e.g. `listIntegrations`, `postMessagesSend`,
  `getConversationMessages`).
- The `description` on each tool is the operation's `summary` +
  `description`, prefixed with the HTTP method and path template.
- The `inputSchema` is a JSON Schema `object` aggregating:
  - every path parameter (always `required`),
  - every query parameter (marked `required` when the spec says so),
  - the request body, presented as an argument named `body` (marked
    `required` when the operation requires a body).

## How to run it

Build the binary:

```bash
make mcp
```

That produces `./bin/nudgeway-mcp`.

Sanity-check the generated tools without an MCP client:

```bash
./bin/nudgeway-mcp --list-tools | jq '.[].name'
```

You should see every operationId from `openapi.yaml`.

### Wire it into Claude Desktop

Add a stanza to `~/Library/Application Support/Claude/claude_desktop_config.json`
(macOS) or the equivalent on your OS:

```json
{
  "mcpServers": {
    "nudgeway": {
      "command": "/absolute/path/to/nudgeway/bin/nudgeway-mcp",
      "env": {
        "NUDGEWAY_API_URL": "http://127.0.0.1:8080",
        "NUDGEWAY_SESSION_COOKIE": "<paste session cookie value>",
        "NUDGEWAY_CSRF_TOKEN": "<paste csrf token value>"
      }
    }
  }
}
```

Restart Claude Desktop. The Nudgeway toolset appears under the tools
menu; the model can now call every REST endpoint on your behalf.

### Wire it into Claude Code

`~/.claude.json` or the project-scoped `.mcp.json`:

```json
{
  "mcpServers": {
    "nudgeway": {
      "command": "./bin/nudgeway-mcp",
      "env": {
        "NUDGEWAY_API_URL": "http://127.0.0.1:8080",
        "NUDGEWAY_SESSION_COOKIE": "…",
        "NUDGEWAY_CSRF_TOKEN": "…"
      }
    }
  }
}
```

## How to authenticate

The MCP server does **not** perform login. It forwards HTTP requests
using credentials you provide via environment variables.

1. Log into the Nudgeway web UI in your browser.
2. Open DevTools → Application → Cookies → `http://localhost:8080`.
3. Copy the `nudgeway_session` cookie value into
   `NUDGEWAY_SESSION_COOKIE`.
4. Copy the `nudgeway_csrf` cookie value (or fetch it via
   `GET /api/v1/auth/csrf`) into `NUDGEWAY_CSRF_TOKEN`. This is required
   for POST / PUT / PATCH / DELETE tool calls.

Environment:

| Variable                  | Default                | Purpose                                          |
| ------------------------- | ---------------------- | ------------------------------------------------ |
| `NUDGEWAY_API_URL`        | `http://127.0.0.1:8080` | Origin of the running Nudgeway server.           |
| `NUDGEWAY_SESSION_COOKIE` | (empty)                | Attached as `Cookie: nudgeway_session=…`.        |
| `NUDGEWAY_CSRF_TOKEN`     | (empty)                | Header `X-CSRF-Token` + `Cookie: nudgeway_csrf`. |

## Tool naming

Tool name = OpenAPI `operationId`. This is the contract: renaming an
operationId is a breaking change for existing MCP clients. When adding a
new endpoint (see CLAUDE.md §8), the tool appears automatically at boot
— no code change in `cmd/mcp` or `internal/mcp` is required.

## Protocol details

- MCP protocol version advertised: `2024-11-05`.
- Framing: newline-delimited JSON over stdio, JSON-RPC 2.0.
- Methods implemented: `initialize`, `ping`, `tools/list`, `tools/call`,
  plus silent handlers for the common `notifications/*` messages.
- Tool-call failures (missing args, non-2xx upstream) come back as
  `isError: true` text content — matching the MCP guidance that tool
  failures are surfaced in-band rather than as JSON-RPC errors.

## Where the code lives

- `cmd/mcp/main.go` — stdio loop + JSON-RPC dispatch.
- `internal/mcp/openapi.go` — YAML → tool descriptor generator.
- `internal/mcp/forward.go` — HTTP forwarder (tool call → REST request).
- `internal/api/openapi/spec.go` — `//go:embed openapi.yaml` accessor.

The package obeys the dependency rule (CLAUDE.md §4): `internal/mcp/*`
imports stdlib + `gopkg.in/yaml.v3` + `internal/api/openapi` only. Zero
provider imports.
