# Groups troubleshooting

## OBA not granted → sync returns 502

**Symptom**: `POST /api/v1/groups/sync` returns `502 provider_error`; the Groups page is empty; [Meta API execution log](/#/audit-telemetry/provider-calls) shows the `list_groups` call failing with a Meta error like "requires official business account".

**Cause**: Meta gates the Business Groups API on OBA phone numbers only.

**Fix**:

1. Open [Settings → Integrations → OBA](/#/integrations/oba-status).
2. Click **Apply** if the status is `NOT_APPLIED`.
3. Wait for `APPROVED` (usually 1–5 business days).
4. Re-run sync.

Any group traffic (inbound webhooks, outbound sends) will begin to work automatically once Meta flips the phone number.

## Member roster looks stale

**Symptom**: The `group_members` page shows people who left, or is missing people who joined.

**Cause**: Sync upserts members but does not tombstone rows Meta no longer returns. A future reconciliation pass will stamp `left_at`; today the roster is best-effort.

**Fix**:

- Re-run sync — new joiners land immediately.
- Departures show as ghosts until the reconciliation pass ships. Compare `SELECT COUNT(*) FROM group_members WHERE group_id = ? AND left_at IS NULL` against the group's `size` field for a rough sanity check.

## Send returns 502 provider_error

**Symptom**: `POST /api/v1/groups/{id}/messages` succeeds locally (202) but the message never transitions past `queued`; the message row's `error` column is populated.

**Diagnosis**: Open [Meta API execution log](/#/audit-telemetry/provider-calls), filter `integration_id=<yours>&operation=send_message`, find the row for the failed send. The Meta response body carries the exact reason.

Common Meta rejections:

- `#131047` — 24-hour customer-service window closed. Send a template instead.
- `#100 (param)` — malformed template payload; verify component structure.
- `#131056 (Business account restriction)` — the WABA is throttled or in review.

## Sync completes but `groups_upserted: 0`

**Cause**: The integration is not a WhatsApp channel, or the WABA genuinely has no groups.

**Check**: `GET /api/v1/integrations/{id}` — `provider` should be `whatsapp` and `capabilities.groups` should be true. If a Zoho / bot / AI integration id was passed, sync returns `422 unsupported`.

## `424 Failed Dependency`

**Cause**: The integration id in the request body does not exist under your org, or has been soft-disconnected.

**Fix**: Re-check the id in Settings → Integrations. A `disconnected` integration ignores sync until you re-run [Test the connection](/#/integrations/test-connection).

## Related

- [Groups overview](/#/groups/overview)
- [OBA status](/#/integrations/oba-status)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
