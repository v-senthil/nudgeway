# Submit for Meta review

Draft templates are local-only. Submitting hands them to Meta for review; the row flips to **PENDING** and — usually within minutes — becomes **APPROVED** or **REJECTED**.

## How to use

1. Go to **Settings → Templates**.
2. Click a row with the **DRAFT** badge.
3. Review the body and check every placeholder count.
4. Click **Submit for review** in the top-right of the detail pane.
5. The row moves to **PENDING**. Refresh the page, or click [**Sync from Meta**](#/templates/sync-from-meta) to force a status check.

You can also submit while creating — tick **Submit for Meta review** on the create form.

## How long review takes

- **Typical** — 5 to 15 minutes for UTILITY and AUTHENTICATION, a bit longer for MARKETING.
- **Worst case** — several hours during Meta review backlogs.

The Templates list does not auto-update template statuses in real time. Refresh, or click **Sync from Meta**, to see the latest state.

## Troubleshooting

- **"Template not in DRAFT state" toast** — the template was already submitted (it's PENDING) or has reached a terminal state (APPROVED or REJECTED). To try again, click **Duplicate**, edit the copy, and submit the copy.
- **"Template not found"** — the template was deleted between opening the list and clicking Submit. Refresh the list.
- **"Target integration missing"** — the WhatsApp integration for this template was disabled. Reconnect it in Settings → Integrations, then submit again.
- **"Meta rejected the submission"** — the reason from Meta shows in the toast. Read it, edit the draft, and submit again. The row stays in DRAFT.
- **Stuck in PENDING for hours** — click **Sync from Meta**. Meta occasionally forgets to push the final status.

## Related

- [Templates overview](#/templates/overview) — full lifecycle.
- [Sync from Meta](#/templates/sync-from-meta) — pull the latest status.
- [Troubleshooting](#/templates/troubleshooting) — rejection reasons and fixes.
