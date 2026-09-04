# Developer

Nudgeway ships four programmable surfaces on top of the same Go modular monolith:

- A **REST API** under `/api/v1/*` — spec-first, RFC 7807 errors, session-cookie or bearer auth.
- An **OpenAPI 3.1 spec** at `internal/api/openapi/openapi.yaml` — the single source of truth. Regenerates Go server interfaces + a TypeScript client on every change.
- A standalone **MCP server** (`cmd/mcp`) that turns every OpenAPI operation into a Model Context Protocol tool — Claude Desktop, Claude Code, Cursor, and any MCP-capable client can drive Nudgeway in natural language.
- A **skills library** under `skills/` — playbooks that Claude Code (and other agent runtimes) auto-discover to drive the API for common workflows.

Everything runs from the single `bin/nudgeway-server` binary. A companion `bin/nudgeway-cli` covers bootstrap tasks (tenant / user / integration / migrate) and `bin/nudgeway-mcp` is the standalone MCP server.

## Change flow (CLAUDE.md §8)

**Spec-first.** Never hand-write handler signatures.

1. Edit `internal/api/openapi/openapi.yaml`. Add request, response, error schemas. Add operation with `operationId`, `security`, `tags`.
2. `make gen-api` regenerates Go server interfaces (`oapi-codegen`) and TS client (`openapi-typescript` + `openapi-fetch`).
3. Implement the handler by satisfying the generated interface in `internal/api/rest/v1/<resource>.go`. Handler calls into `internal/application/*` — never directly into `internal/infrastructure/*` or `internal/providers/*`.
4. Add contract test asserting the real response validates against the OpenAPI schema.
5. Update `docs/api/CHANGELOG.md`.
6. Wire the frontend hook using the generated client.

The MCP server picks up the new operation automatically on the next `make mcp` — no manual tool registration.

## Where to go next

- [REST API](/#/developer/rest-api) — base URL, auth modes, CSRF, error shape.
- [OpenAPI spec](/#/developer/openapi) — source of truth, generation flow, browsable Scalar reference.
- [MCP server](/#/developer/mcp-server) — build, run, wire.
- [Claude Desktop setup](/#/developer/claude-desktop) — full config example + troubleshooting.
- [Skills library](/#/developer/skills) — the playbooks bundled under `skills/`.
- [CLI (nudgeway-cli)](/#/developer/cli) — tenant / user / integration / migrate commands.
