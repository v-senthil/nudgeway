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
        "NUDGEWAY_API_TOKEN": "nk_abcd1234_<40-char-secret>"
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
        "NUDGEWAY_API_TOKEN": "nk_abcd1234_<40-char-secret>"
      }
    }
  }
}
```

## How to authenticate

The MCP server does **not** perform login. It forwards HTTP requests
using credentials you provide via environment variables. Two modes are
supported; **prefer API tokens** unless you're doing throwaway local
poking with an existing browser session.

### API tokens (recommended)

Tokens are opaque bearer credentials in the shape
`nk_<8-char-prefix>_<40-char-secret>` (base32). The prefix is stored
plaintext and used for lookup; the secret is `argon2id`-hashed at rest.
The full token is shown **exactly once**, at creation time.

Behaviour of the bearer path:

- Sent as `Authorization: Bearer <token>` on every request.
- **No CSRF header required.** The backend's bearer middleware
  short-circuits the CSRF double-submit check when a token is present.
- The MCP forwarder skips `Cookie: nudgeway_session=…` and
  `Cookie: nudgeway_csrf=…` entirely when `NUDGEWAY_API_TOKEN` is set.
- Tokens carry the same RBAC scopes as the user that minted them and
  are revocable from `/settings/api-tokens`.

Set the env var:

```bash
export NUDGEWAY_API_TOKEN=nk_abcd1234_<40-char-secret>
./bin/nudgeway-mcp
```

On startup the server logs the active auth mode to stderr:

```
nudgeway-mcp: base=http://127.0.0.1:8080 auth=api-token tools=23
```

### How to mint an API token

**Via the UI** — recommended for humans:

1. Sign into Nudgeway.
2. Go to `/settings/api-tokens`.
3. Click *New token*, pick a name (e.g. `claude-desktop-laptop`) and an
   optional expiry.
4. Copy the displayed plaintext token into your MCP client config. It
   will not be shown again — if you lose it, revoke and create a new
   one.

**Via the MCP tool** — recommended for agents already talking to a live
Nudgeway (bootstrapping a second client, rotating tokens, etc.):

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

The response body contains the plaintext `token` field once. Companion
tools: `listAPITokens`, `revokeAPIToken`.

### Session cookie (fallback for local dev)

Only use this path when you don't yet have a token minted and you need
to poke the API from a fresh browser login.

1. Log into the Nudgeway web UI in your browser.
2. Open DevTools → Application → Cookies → `http://localhost:8080`.
3. Copy the `nudgeway_session` cookie value into
   `NUDGEWAY_SESSION_COOKIE`.
4. Copy the `nudgeway_csrf` cookie value (or fetch it via
   `GET /api/v1/auth/csrf`) into `NUDGEWAY_CSRF_TOKEN`. This is required
   for POST / PUT / PATCH / DELETE tool calls.

Cookies are 30-day sliding; refresh when they expire. If
`NUDGEWAY_API_TOKEN` is also set, the cookie values are ignored.

### Environment reference

| Variable                  | Default                 | Purpose                                                                     |
| ------------------------- | ----------------------- | --------------------------------------------------------------------------- |
| `NUDGEWAY_API_URL`        | `http://127.0.0.1:8080` | Origin of the running Nudgeway server.                                      |
| `NUDGEWAY_API_TOKEN`      | (empty)                 | **Preferred.** Sent as `Authorization: Bearer …`; skips session + CSRF.     |
| `NUDGEWAY_SESSION_COOKIE` | (empty)                 | Fallback. Attached as `Cookie: nudgeway_session=…` when no token is set.    |
| `NUDGEWAY_CSRF_TOKEN`     | (empty)                 | Fallback. Header `X-CSRF-Token` + `Cookie: nudgeway_csrf` on state changes. |

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
