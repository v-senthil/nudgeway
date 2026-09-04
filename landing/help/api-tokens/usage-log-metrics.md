# Usage log and metrics

Every request a token makes is captured, so you can see exactly what each client is doing. This is the surface to open when you want to answer "is this token still in use?", "what did the vendor's integration do last night?", or "why is this client suddenly getting errors?".

## How to use

1. Go to **Settings → API tokens**.
2. Click any token row. A drawer opens on the right with two tabs.

### Overview tab

- **KPI cards** show the token's call count, error rate, and typical latency (p50 / p95) for the selected time window.
- A **per-day sparkline** shows request volume across the window.
- **Top paths** lists which parts of Nudgeway the token has been hitting most.

Use this tab to check a token is healthy at a glance. A flat sparkline and a stale **Last used** value usually mean the client is no longer running.

### Log tab

- A reverse-chronological table of every request the token has made: time, source IP, what was called, the response code, and how long it took.
- Filters at the top let you narrow by method, by status class (successes, client errors, server errors), or by a substring of the path.
- Click any row to expand it and see the request and response bodies. Sensitive fields such as passwords, tokens, and secrets are automatically replaced with `[redacted]` before storage.

## What gets stored, what doesn't

- Nudgeway keeps the first 8 KiB of each request and response body. Anything larger is truncated, but the total size is still recorded.
- Binary payloads (file uploads, media downloads) are not stored — you'll see the size but not the content.
- Sensitive keys are redacted automatically, so screenshots from the drawer are safe to share when you need to ask for help.

## Troubleshooting

- **The drawer is empty for a token I just created.** The token hasn't been used yet. Once the client makes its first request, rows appear within a few seconds.
- **Some rows show a redacted body.** Expected — Nudgeway strips passwords, tokens, and secret-shaped fields before storing anything.
- **Some rows show no body at all.** The payload was larger than 8 KiB or was binary. The size column still tells you how big it was.
- **Metrics look empty but the log has rows.** The KPI cards may take a moment to catch up on a brand-new token. Refresh the drawer after a minute.
- **I'm seeing requests I don't recognise.** Check the source IP and the top paths. If it doesn't match the client you handed the token to, revoke immediately and mint a fresh one — see [Revoke a token](#/api-tokens/revoke-token).

## Related

- [Overview](#/api-tokens/overview)
- [Create a token](#/api-tokens/create-token)
- [Revoke a token](#/api-tokens/revoke-token)
- [Troubleshooting](#/api-tokens/troubleshooting)
