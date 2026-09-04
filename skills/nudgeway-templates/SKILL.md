---
name: nudgeway-templates
description: WhatsApp message templates — list, create, sync-from-Meta, submit for review. Templates are the only messages allowed outside the 24h customer-service window.
trigger: User asks about WhatsApp templates, template review status, syncing templates from Meta, or sending a template message.
---

# Nudgeway templates skill

## Overview

WhatsApp templates are pre-approved message shapes (header / body / footer / buttons) that Meta reviews before use. Templates unlock outbound-first messaging (marketing, transactional notifications). Nudgeway mirrors template state from Meta, submits new ones, and renders them WhatsApp-native in the thread.

## Surface (not yet in openapi.yaml — call REST directly)

```
GET    /api/v1/templates              — list org's templates
POST   /api/v1/templates              — create + submit for review
POST   /api/v1/templates/sync         — pull latest state from Meta
GET    /api/v1/templates/{id}         — one template + review status
```

Once these operations land in openapi.yaml, they will auto-appear as MCP tools with operationIds like `listTemplates`, `createTemplate`, `syncTemplates`, `getTemplate`.

## Patterns

### Send a template message

Templates are sent through the normal messages endpoint with `type: "template"`:

```json
{
  "tool": "postMessagesSend",
  "arguments": {
    "body": {
      "conversation_id": "...",
      "type": "template",
      "template": {
        "name": "order_shipped",
        "language": "en",
        "components": [
          {
            "type": "body",
            "parameters": [
              { "type": "text", "text": "Aditi" },
              { "type": "text", "text": "#4821" }
            ]
          }
        ]
      }
    }
  }
}
```

The backend substitutes parameters and persists a `metadata.template.resolved` view so the inbox renders it WhatsApp-style (header / body / footer / button chips).

## Gotchas

- **Review lag**: `status: pending` immediately after submit. Meta typically approves within minutes but can take hours. Use `POST /templates/sync` to refresh.
- **Category constraints**: `AUTHENTICATION` / `MARKETING` / `UTILITY` — templates rejected because of category mismatch require a new submission (not an edit).
- **Placeholder syntax**: positional `{{1}} {{2}}` or named `{{name}}`. The backend handles both.
- **Buttons**:  Meta supports `URL`, `QUICK_REPLY`, `PHONE_NUMBER`, `COPY_CODE`, `VOICE_CALL`. All render as chips in Nudgeway's TemplateBubble.

## Related skills

- [`nudgeway-inbox`](../nudgeway-inbox/SKILL.md) — templates are sent via `postMessagesSend`.
- [`nudgeway-integrations`](../nudgeway-integrations/SKILL.md) — template sync requires a connected integration with `whatsapp_business_management` scope.
