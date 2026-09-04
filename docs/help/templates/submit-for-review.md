# Submit for Meta review

`DRAFT` templates are local-only. Submitting hands them to Meta for review; the row flips to `PENDING` and — usually within minutes — becomes `APPROVED` or `REJECTED`.

## How to use

1. Templates → click a `DRAFT` row.
2. Review the body + parameters.
3. Click **Submit for review**.
4. The row moves to `PENDING`. Refresh, or click [**Sync from Meta**](/#/templates/sync-from-meta) to force-refresh the status.

You can also submit inline while creating — pass `submit: true` on [`createTemplate`](/#/templates/create).

## API

**operationId**: `submitTemplate`

```
POST /api/v1/templates/{id}/submit
```

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/templates/01M1…/submit' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>'
```

Response is a full `Template` object reflecting the provider-reported state after submission (usually `PENDING`, occasionally `APPROVED` inline for simple utilities).

## Review lag

- **Typical** — 5-15 minutes for `UTILITY` and `AUTHENTICATION`, a bit longer for `MARKETING`.
- **Worst case** — several hours during Meta review backlog windows.
- **Automation-safe** — poll [`syncTemplates`](/#/templates/sync-from-meta) every 1-2 minutes if you need programmatic notification of the flip. The UI does not push template status via WebSocket; a browser refresh or an explicit sync fetches the latest.

## MCP

Call the `submitTemplate` tool with `{ "id": "<template-ULID>" }`.

## Troubleshooting

- **`409 template not in DRAFT state`** — already submitted (`PENDING`) or in a terminal state (`APPROVED` / `REJECTED`). Duplicate + resubmit if you need a new attempt.
- **`404 template not found`** — the ULID is wrong or was deleted. Re-list via `getTemplates`.
- **`424 target integration missing`** — the integration was disabled between draft-save and submit. Reconnect via [Integrations](/#/integrations/connect-whatsapp).
- **`502 provider rejected the submission`** — Meta rejected inline (bad shape, banned word, etc.). Read the response `detail`; the row stays in `DRAFT`.
- **Stuck in `PENDING` for hours** — force a [Sync from Meta](/#/templates/sync-from-meta). Meta occasionally doesn't push the terminal-state webhook.

## Related

- [Templates overview](/#/templates/overview) — full lifecycle.
- [Sync from Meta](/#/templates/sync-from-meta) — pull latest state.
- [Troubleshooting](/#/templates/troubleshooting) — rejection reasons + fixes.
