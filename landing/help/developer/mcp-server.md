# MCP server

`cmd/mcp` is a standalone Go binary that reads `internal/api/openapi/openapi.yaml` at boot, generates one MCP tool per operation (auto-derived JSON Schema from path / query / body params), and forwards tool calls to the running Nudgeway REST API. It speaks MCP protocol version `2024-11-05` over stdio.

Any MCP-capable client — Claude Desktop, Claude Code, Cursor, custom SDKs — can drive the entire Nudgeway API in natural language once the server is wired up.

## Build

```bash
make mcp
```

Produces `./bin/nudgeway-mcp` (~6.6 MB, static). The spec is embedded via `//go:embed openapi.yaml`, so the binary is fully self-contained.

## Inspect the generated tools

```bash
./bin/nudgeway-mcp --list-tools | jq '.[].name'
```

You should see every OpenAPI `operationId` — `listIntegrations`, `postMessagesSend`, `getConversationMessages`, `createAPIToken`, `revokeAPIToken`, and so on. Tool name equals operationId verbatim; renaming an operationId is a breaking change for existing MCP clients.

## Run + wire it in

See [Claude Desktop setup](/#/developer/claude-desktop) for the full JSON config. Shape:

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

On startup the server logs the active auth mode to stderr:

```
nudgeway-mcp: base=http://127.0.0.1:8080 auth=api-token tools=73
```

## Environment variables

| Variable                  | Default                 | Purpose                                                                     |
| ------------------------- | ----------------------- | --------------------------------------------------------------------------- |
| `NUDGEWAY_API_URL`        | `http://127.0.0.1:8080` | Origin of the running Nudgeway server.                                      |
| `NUDGEWAY_API_TOKEN`      | (empty)                 | **Preferred.** Sent as `Authorization: Bearer …`; skips session + CSRF.     |
| `NUDGEWAY_SESSION_COOKIE` | (empty)                 | Fallback. Attached as `Cookie: nudgeway_session=…` when no token is set.    |
| `NUDGEWAY_CSRF_TOKEN`     | (empty)                 | Fallback. Header `X-CSRF-Token` + `Cookie: nudgeway_csrf` on state changes. |

## Auth precedence

The MCP server does **not** perform login. It forwards HTTP requests using whatever credentials you supply via env vars.

- **`NUDGEWAY_API_TOKEN` wins.** When set, the forwarder sends `Authorization: Bearer <token>` and drops any session / CSRF cookie values. The backend's bearer middleware short-circuits the CSRF double-submit check.
- **Session cookie is the fallback.** Only used when `NUDGEWAY_API_TOKEN` is empty. On state-changing methods (`POST` / `PUT` / `PATCH` / `DELETE`) the forwarder sets both `X-CSRF-Token: <NUDGEWAY_CSRF_TOKEN>` and the `nudgeway_csrf` cookie — matching the backend's expectation in `internal/infrastructure/auth/csrf.go`.

Prefer API tokens for anything other than throwaway local poking with an existing browser session. See [Create a token](/#/api-tokens/create-token).

## Protocol details

- MCP protocol version advertised: `2024-11-05`.
- Framing: newline-delimited JSON over stdio, JSON-RPC 2.0.
- Methods implemented: `initialize`, `ping`, `tools/list`, `tools/call`, plus silent handlers for common `notifications/*` messages.
- Tool-call failures (missing args, non-2xx upstream) come back as `isError: true` text content — matching the MCP guidance that tool failures are surfaced in-band rather than as JSON-RPC errors.

## Where the code lives

- `cmd/mcp/main.go` — stdio loop + JSON-RPC dispatch.
- `internal/mcp/openapi.go` — YAML → tool descriptor generator.
- `internal/mcp/forward.go` — HTTP forwarder (tool call → REST request).
- `internal/api/openapi/spec.go` — `//go:embed openapi.yaml` accessor.

The package obeys the dependency rule (CLAUDE.md §4): `internal/mcp/*` imports stdlib + `gopkg.in/yaml.v3` + `internal/api/openapi` only. Zero provider imports.

## Related

- [Claude Desktop setup](/#/developer/claude-desktop) — full config walkthrough.
- [OpenAPI spec](/#/developer/openapi) — the source the tool set is generated from.
- [API tokens overview](/#/api-tokens/overview) — preferred auth mode.
- [Skills library](/#/developer/skills) — playbooks paired with the MCP tools.
