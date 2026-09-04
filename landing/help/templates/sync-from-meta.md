# Sync status from Meta

Templates are mirrored from Meta. Meta pushes status changes automatically, but pushes can be missed (throttled, dropped, or if webhooks were disabled). **Sync from Meta** reconciles the local list by pulling every template from Meta and updating what we have.

## When to use it

- A **PENDING** template has been stuck for more than a few minutes.
- You just connected a fresh integration and want to pull existing templates that were created outside Nudgeway.
- You suspect a webhook gap (network issue, disabled webhook, tunnel down).
- You just want to see the latest state.

## What can change on sync

- **PENDING → APPROVED** — sendable in the composer.
- **PENDING → REJECTED** — Meta returned a reason; open the template to see it.
- **APPROVED → PAUSED** — Meta throttled it for quality reasons.
- **APPROVED → DISABLED** — Meta stopped accepting sends against it.

Local **DRAFT** rows Meta doesn't know about are left alone — you won't lose in-progress drafts.

## How to use

1. Go to **Settings → Templates**.
2. Pick the integration in the filter dropdown (Sync is per-integration).
3. Click **Sync from Meta** in the top-right.
4. A spinner runs while the sync is in flight. A toast confirms how many templates were fetched and updated.
5. The list refreshes with the new statuses.

## Troubleshooting

- **"Missing integration" toast** — you didn't pick an integration before clicking Sync. Pick one from the filter dropdown.
- **Toast says "0 fetched"** — either the WhatsApp Business Account has no templates, or the access token doesn't have the management scope. Go to Settings → Integrations, open the integration, and click **Test connection** — if it fails, re-enter the token with the correct scopes.
- **"Provider error"** — Meta had a transient error. Wait 30 seconds and click Sync again.
- **Statuses look stale even after Sync succeeded** — refresh the page. The list occasionally caches an old snapshot.

## Related

- [Templates overview](#/templates/overview) — lifecycle.
- [Submit for Meta review](#/templates/submit-for-review) — the flow that creates a PENDING.
- [Troubleshooting](#/templates/troubleshooting) — rejection and status issues.
