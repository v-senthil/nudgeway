# OpenAPI spec

`internal/api/openapi/openapi.yaml` is the **single source of truth** for the Nudgeway HTTP surface. The Go server interfaces, the TypeScript client, and the MCP tool set are all generated from it. Handlers are never hand-wired.

The spec is embedded into every binary via `//go:embed openapi.yaml` (`internal/api/openapi/spec.go`), so builds are self-contained — no external YAML at runtime.

## Current shape

- OpenAPI **3.1.0**.
- Info title: `Nudgeway API`.
- **73+ operations** across `system`, `auth`, `contacts`, `conversations`, `messages`, `integrations`, `webhooks`, `provider-calls`, `audit`, `api-tokens`, `templates`, `groups`, `calls`, `analytics`, `meta-analytics`, `tools`.
- Errors on every operation follow RFC 7807 (`application/problem+json`) via a shared `Problem` schema.
- Two `securitySchemes`: `sessionCookie` and `apiKey` (bearer).

## Browse it

Nudgeway ships an interactive reference at:

```
http://localhost:8080/api.html
```

This is a static Scalar renderer (`landing/api.html`) that loads `landing/openapi.yaml`. It gives you searchable operation navigation, request / response examples, and a "try it" panel.

## Change flow (CLAUDE.md §8)

**Spec-first.** No exceptions.

1. Edit `internal/api/openapi/openapi.yaml`. Add or modify request / response / error schemas, and the operation with `operationId`, `security`, and `tags`.
2. Run `make gen-api`. This regenerates:
   - Go server interfaces via `oapi-codegen` (server-side handler contracts + typed models).
   - TypeScript client via `openapi-typescript` + `openapi-fetch` (typed hooks for the frontend).
3. Implement the handler in `internal/api/rest/v1/<resource>.go` by satisfying the generated interface. The handler must call into `internal/application/*` — never directly into `internal/infrastructure/*` or `internal/providers/*`.
4. Add a contract test asserting a real response validates against the OpenAPI schema.
5. Update `docs/api/CHANGELOG.md` and bump the `info.version` line.
6. Wire the frontend using the generated client hook.

The MCP server picks up the new operation automatically on the next `make mcp` — no manual tool registration.

## Conventions in the spec

- Every `operationId` is `camelCase` and unique across the whole spec. It doubles as the MCP tool name — renaming an operationId is a **breaking change** for MCP clients.
- Every operation declares `security` explicitly. `system` endpoints use `security: []` (public probes). Everything else lists `sessionCookie` and/or `apiKey`.
- Path templates use `{id}`-style names — matching how the API token usage log records them (bounds cardinality on the `path` column).
- Errors are attached per-response with `content: application/problem+json: schema: {$ref: '#/components/schemas/Problem'}`.

## Guards

- `spectral` lints the spec on every CI run (`make verify`).
- `openapi-diff` blocks accidental breaking changes.
- `sqlc-diff` catches migrations that drift from queries.

## Related

- [REST API](/#/developer/rest-api) — runtime auth + error shape.
- [MCP server](/#/developer/mcp-server) — auto-generated tool set.
- [Overview](/#/developer/overview) — the developer surface at a glance.
