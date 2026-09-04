# Skills library

The `skills/` directory ships domain playbooks that agent runtimes (Claude Code, and any client that understands the Skill format) auto-discover. Each skill wraps one slice of the MCP tool surface with concrete recipes, common patterns, and gotchas — so the model has strong priors instead of guessing.

## What ships

```
skills/
  README.md
  nudgeway-analytics/
  nudgeway-api-tokens/
  nudgeway-calls/
  nudgeway-inbox/
  nudgeway-integrations/
  nudgeway-mcp/
  nudgeway-templates/
```

Each subdirectory contains a `SKILL.md`. Highlights:

- **`nudgeway-mcp`** — bring up the MCP server; wire Claude Desktop / Claude Code / Cursor; auth precedence.
- **`nudgeway-api-tokens`** — mint / list / revoke bearer credentials; fetch usage log + metrics.
- **`nudgeway-integrations`** — connect a WhatsApp integration; verify signature; push webhook to Meta.
- **`nudgeway-inbox`** — list conversations, send text / media / templates, mark read.
- **`nudgeway-templates`** — draft, submit for Meta review, sync status.
- **`nudgeway-calls`** — inbound popup + WebRTC accept, outbound, recording / transcript.
- **`nudgeway-analytics`** — Nudgeway KPIs + Meta Analytics passthrough.

The `README.md` at the top is the index — start there.

## Format

Every skill file is a Markdown document with YAML frontmatter:

```markdown
---
name: nudgeway-api-tokens
description: Nudgeway API tokens — mint, list, and revoke opaque bearer credentials …
trigger: User asks about API keys, API tokens, bearer credentials, personal access tokens, wiring the MCP server auth, or replacing session-cookie auth for scripted API access.
---

# Nudgeway API tokens skill

## Overview
…

## MCP tools
…

## Common patterns
…

## Gotchas
…

## Related skills
…
```

The `trigger` field is the natural-language predicate the runtime matches against the user's ask.

## Auto-discovery in Claude Code

Claude Code walks `skills/` at session start and surfaces each skill in the `/skills` list. When the user's message matches a skill's `trigger`, the runtime loads the full `SKILL.md` body into context before responding — no manual `/load` needed.

To trigger explicitly, ask for the skill by name (e.g. "use the nudgeway-mcp skill to wire Claude Desktop") or invoke the runtime's skill command directly.

## Loading a skill by hand

If you're on a different MCP client or want to seed context yourself, paste the relevant `SKILL.md` body into the conversation. The Overview + Common patterns sections are typically enough for the model to drive the tool set correctly.

## Add a new skill

1. Create `skills/nudgeway-<slice>/SKILL.md`.
2. Fill in `name`, `description`, `trigger` in the frontmatter.
3. Structure the body: Overview → MCP tools / REST equivalents → Common patterns → Gotchas → Related skills.
4. Add a line to `skills/README.md`.
5. Reference the skill from any help page that covers the same territory.

## Related

- [MCP server](/#/developer/mcp-server) — the tool set every skill drives.
- [Claude Desktop setup](/#/developer/claude-desktop) — wire the runtime that consumes skills.
- [OpenAPI spec](/#/developer/openapi) — source of truth for what tools exist.
