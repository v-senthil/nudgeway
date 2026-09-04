# Meta API execution log

Every outbound HTTP call Nudgeway made to Meta shows up here, one row per round-trip. This is the go-to place when someone asks "why did that message fail?" or "what did Meta actually return?".

## Columns you see

| Column | Notes |
|---|---|
| **Occurred at** | Timestamp of the call. |
| **Integration** | Which integration the call was made on behalf of. |
| **Operation** | What Nudgeway was trying to do — see the list below. |
| **Method / URL** | The HTTP method and the fully-qualified Meta URL. Never contains secrets. |
| **Status** | HTTP status code Meta returned. `0` means Nudgeway never reached Meta (network error). |
| **Latency** | How long the call took in milliseconds. |
| **Request body / Response body** | The exact payload sent and the exact reply. Click a row to expand these. |

## Operations you'll see

| Operation | What it maps to on Meta |
|---|---|
| `send_message` | Sending a WhatsApp message. |
| `mark_as_read` | Marking a customer's message as read. |
| `get_media_url` | Fetching a downloadable URL for a media attachment. |
| `download_media` | Downloading media bytes. Body column is empty by design — the bytes stream to disk, not to this log. |
| `upload_media` | Uploading media before sending it as an attachment. |
| `list_templates` | Fetching the WABA's template list. |
| `create_template` | Submitting a new template for approval. |
| `get_template_status` | Checking a submitted template's review state. |
| `list_groups` / `get_group` | Group sync. |
| `health_check` | The Test connection button. |

## Redaction

Nudgeway automatically replaces sensitive values in request bodies with the string `[redacted]` before storing them. This covers passwords, access tokens, app secrets, verify tokens, and any field named "token", "secret", or similar. Non-text responses (media downloads) store `null` for the body and only record the byte size. Request and response bodies are capped at 64 KiB — anything larger is truncated.

## How to use

1. Click **Settings** -> **Audit** -> **Meta API execution log** tab.
2. Use the filters at the top:
   - **Integration** to scope to one integration.
   - **Operation** to scope to one kind of call (e.g. `send_message`).
   - **Status range** — set the minimum to 400 and the maximum to 599 to show failures only.
   - **Time range** with the date picker.
3. Click any row to expand its request and response body panels.

## Troubleshooting

- **A call you expected isn't in the list** — for `download_media` calls, the response body is intentionally empty because bytes stream to disk. If a `send_message` you know happened is truly missing, contact your admin — a small number of legacy code paths don't yet write here.
- **Body shows `[redacted]`** — that's the redactor doing its job. The sensitive field will never appear in this log; that's by design.
- **Status shows 0** — the request never made it to Meta. Common cause is a network problem or DNS failure between Nudgeway and Meta. Retry the original action; if it happens repeatedly, contact your admin.
- **Body is cut off at 64 KiB** — the log truncates very large bodies. For debugging a specific gigantic payload, re-run the original action and capture the full body in your browser's network tab, or ask an admin to inspect Meta directly.
- **Latency spike on many rows** — Meta is slow or throttling. Wait a few minutes and re-check. If it persists, an admin should compare timings against Meta's status page.

## Related

- [Audit & Meta telemetry overview](#/audit-telemetry/overview)
- [Audit log](#/audit-telemetry/audit-log)
- [Integrations overview](#/integrations/overview)
