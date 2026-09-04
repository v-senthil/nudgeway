# Send a template message

Templates are the only messages allowed outside WhatsApp's 24-hour customer-service window. They are pre-approved by Meta and rendered by the recipient's WhatsApp client with the exact layout (header / body / footer / buttons) that was approved.

## When to use

- Any outbound reply more than 24 hours after the customer's last inbound.
- The first-touch of an outbound-initiated conversation (marketing, transactional notifications).
- Any send where the recipient must opt back in via a button (`QUICK_REPLY`, `URL`, `PHONE_NUMBER`, `COPY_CODE`, `VOICE_CALL`).

If you send a `type: text` message outside the 24h window, `postMessagesSend` returns `422 outside 24h window` — the composer switches to the template picker automatically in that state.

## How to use

1. Open a conversation past its 24h window; the composer shows a **Send template** button.
2. Pick a template from the picker (only `APPROVED` templates for the conversation's integration appear).
3. Fill in the parameters. Placeholders are either **positional** (`{{1}} {{2}}`) or **named** (`{{name}}`) — the backend accepts both.
4. Hit **Send**. The backend substitutes parameters, submits to Meta, and persists a `metadata.template.resolved` view so the inbox renders the message WhatsApp-style (header, body, footer, button chips).

## API

**operationId**: `postMessagesSend` with `type: template`

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/messages' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "conversation_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
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
    },
    "idempotency_key": "tmpl-01M1…"
  }'
```

Response is the same `SendMessageAccepted` shape as text (message queued, 202).

Header parameters accept `text`, `image`, `video`, `document`, or `location`. Button parameters accept `payload` (for `QUICK_REPLY`) or `text` (for URL suffix parameters).

## MCP

Call the `postMessagesSend` tool with `type: template` and a fully-formed `template` object — the same shape as the REST body above.

## Troubleshooting

- **`422 template not found`** — the name is misspelled, or the template isn't `APPROVED` for this integration. Run [Sync from Meta](/#/templates/sync-from-meta) to refresh.
- **`422 parameter count mismatch`** — the number of parameters passed doesn't match the placeholders in the approved body. Re-check the template body in the [Templates](/#/templates/overview) page.
- **Bubble renders as plain text on the customer's phone** — you sent `type: text` with template-like content. Use `type: template` and pass the components.
- **`language` rejected** — Meta locale, not just BCP47. Use `en`, `en_US`, `pt_BR`, etc. The template's approved language must match exactly.
- **Buttons don't appear on the recipient's WhatsApp** — button components require a `button` component per button, in order, with the correct `sub_type` (`quick_reply`, `url`, ...). The backend forwards components verbatim to Meta.

## Related

- [Templates overview](/#/templates/overview) — lifecycle + review.
- [Create a template](/#/templates/create) — build a new one.
- [Send a text message](/#/inbox/send-text) — the in-window path.
