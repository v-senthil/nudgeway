# Nudgeway skills

Skills are per-domain playbooks that describe *what an LLM can do with Nudgeway* and *which tools to invoke*. Each skill maps to a slice of the OpenAPI surface (and therefore to a subset of `nudgeway-mcp` tools, since MCP tools are auto-generated from that spec).

## When to use these

- You're an agent working against a live Nudgeway instance via the MCP server (`./bin/nudgeway-mcp`).
- You're an agent writing code against the REST API directly (`curl`, `openapi-fetch`, etc.).
- You need to explain to a user what Nudgeway can do without opening the OpenAPI spec.

## Loading

- **Claude Code**: skills in `skills/*/SKILL.md` are auto-discovered when this repo is the working directory. Invoke with `/skill <name>` or reference by name in-context.
- **MCP clients** (Claude Desktop, Cursor, other): the skills are informational — read the SKILL.md, then call the MCP tools by `operationId`.
- **Any LLM**: paste the relevant SKILL.md into the system prompt.

## Index

| Skill | Domain | Key operations |
|---|---|---|
| [`nudgeway-inbox`](nudgeway-inbox/SKILL.md) | Live WhatsApp inbox — conversations, messages, read state | `getConversations`, `getConversationMessages`, `postMessagesSend`, `postConversationMarkRead` |
| [`nudgeway-integrations`](nudgeway-integrations/SKILL.md) | Provider integrations — create / test / delete / set webhook | `listIntegrations`, `createIntegration`, `testIntegration`, `deleteIntegration` |
| [`nudgeway-templates`](nudgeway-templates/SKILL.md) | WhatsApp message templates — CRUD + Meta sync | Templates surface (see skill) |
| [`nudgeway-calls`](nudgeway-calls/SKILL.md) | Call flow — inbound popup, recording, transcript | Calls surface (see skill) |
| [`nudgeway-analytics`](nudgeway-analytics/SKILL.md) | KPI cards + sparklines — messages / delivery rate / calls | Analytics surface (see skill) |
| [`nudgeway-api-tokens`](nudgeway-api-tokens/SKILL.md) | API tokens — mint / list / revoke bearer credentials for MCP + scripted access | `createAPIToken`, `listAPITokens`, `revokeAPIToken` |
| [`nudgeway-mcp`](nudgeway-mcp/SKILL.md) | Meta-skill — how to bring up the MCP server + wire it into a client | `./bin/nudgeway-mcp` |

## Anatomy of a skill file

```
---
name: nudgeway-<domain>
description: <one-liner an LLM uses to decide whether to load this skill>
trigger: <when the user's ask matches this skill>
---

# Overview
<2 sentences>

# MCP tools
<operationId list with 1-line description>

# REST equivalents
<HTTP verb + path>

# Patterns
<2–3 short recipes with tool call examples>

# Gotchas
<edge cases, auth quirks, tenancy rules>
```

## Golden rules (apply to every skill)

1. **Multi-tenant**. Every write is org-scoped by the caller's session — never trust an `organization_id` from the client.
2. **CSRF**. Session-cookie state-changing calls (`POST`, `PUT`, `DELETE`) require the `X-CSRF-Token` header matching the `nudgeway_csrf` cookie. The MCP forwarder handles this automatically when `NUDGEWAY_CSRF_TOKEN` is set. **API-token requests skip CSRF** — the backend's bearer middleware accepts state-changing calls without a double-submit.
3. **Idempotency**. Send-message accepts a `client_reference_id`; reuse it to avoid duplicate sends on retry.
4. **RBAC**. Every route checks a per-permission gate — `integrations.manage`, `messages.send`, `calls.read`, etc. Requests without the permission return 403.
5. **Audit trail**. Every mutation writes an `AuditLog` row visible at `/settings/audit` and via `getAuditLogs`.
