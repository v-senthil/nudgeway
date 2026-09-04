# Analytics troubleshooting

## Nudgeway tab — all cards show 0

**Symptom**: Every KPI card renders `—` or `0`. Sparklines are flat.

**Most common cause**: The rollup worker has not yet processed the range.

**Diagnosis**:

1. The rollup runs every 15 minutes. If you just imported traffic or restarted the server, wait one tick.
2. Query `analytics_rollup_state` — the `analytics.rollup.daily` bookmark tells you the last processed day.
3. Confirm the raw tables have rows: `SELECT COUNT(*) FROM messages WHERE created_at >= NOW() - INTERVAL 1 DAY;`

**Fix**:

- Restart the server for an immediate boot tick.
- Confirm the range picker is not set to a future date.
- Confirm your server's system clock is roughly aligned with UTC — the runner processes yesterday + today + tomorrow (local) but the REST API takes UTC-anchored ISO dates. A 5-hour clock skew can hide a day of data.

## Nudgeway tab — sparkline sitting at a flat "max 2"

**Cause**: The chart has a floor of 1 with 15% headroom. All-zero data reads as "max 2" — this is a visual convention, not real data.

**Fix**: Send a real message and wait 15 minutes.

## Meta Analytics — pricing tier column is empty

**Cause**: Meta omits the `tier` field on free-tier / free-entry-point / free-customer-service pricing types. Not a bug.

**Fix**: Filter `pricing_types=REGULAR` to see only rows Meta bills.

## Meta Analytics — 400 from Meta

**Symptom**: A sub-section shows the red error banner with a Meta detail like "`(#100) The parameter phone_numbers is required`".

**Cause**: Some Meta endpoints reject requests without at least one dimension or filter set.

**Fix**:

- Ensure the integration picker has resolved — the `phone_numbers` default comes from `PhoneNumber.display_phone_number`, which is loaded via `getIntegrationPhoneNumber`. If that call is still pending or failed, the filter is blank.
- Type an E.164 number (no `+`) into the phone_numbers box manually.
- Widen the range: some Meta endpoints reject sub-hour windows outside `HALF_HOUR` granularity.

## Meta Analytics — integration picker doesn't switch

**Symptom**: You choose a different integration in the drop-down but the sub-sections keep showing the old data.

**Diagnosis**: The picker is bound to the query key `['meta-analytics', section, integrationID, range, granularity, phoneNumber]`. If TanStack Query has a fresh cache entry for the old key it re-uses it.

**Fix**:

- Change the range to force a new query key.
- Hard-refresh the tab (Cmd/Ctrl-Shift-R).
- Check the browser console for a fetch that returned `502` — the tab keeps the last successful data around when a refresh fails.

## Both tabs — 401 Unauthorized

Your session cookie has expired. Log back in.

## Both tabs — 403 Missing analytics.read

Your role does not include the `analytics.read` permission. Ask an org admin to grant it via the CLI (`nudgeway-cli role grant …`) or SQL.

## Related

- [Analytics overview](/#/analytics/overview)
- [Nudgeway tab](/#/analytics/nudgeway-tab)
- [Meta Analytics tab](/#/analytics/meta-analytics-tab)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
