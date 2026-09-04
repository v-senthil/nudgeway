# Flow — Analytics rollup

The rollup pipeline turns rows in `messages` + `conversations` into
per-day aggregates in `analytics_*_daily`. It runs off-request on a
15-minute ticker inside the same server binary.

## Sequence

```
+-------------------+       +-----------------------+
| AnalyticsRunner   | tick  | application/analytics |
|  (Interval=15m)   +-----> |  Service.Rollup(...)  |
+-------------------+       +----------+------------+
                                       |
                    +------------------+------------------+
                    v                                     v
        +-----------------------+          +-----------------------+
        | AnalyticsSource       |          | AnalyticsRepo         |
        | CountMessagesByDay    |          | UpsertMessagesDaily   |
        | CountConversationsBy… |          | UpsertConversations…  |
        | P50ResponseTimeByDay  |          | UpsertDeliveryRate…   |
        +-----------+-----------+          +-----------+-----------+
                    |                                  |
                    v                                  v
              messages,                        analytics_messages_daily,
              conversations                    analytics_conversations_daily,
              (MySQL — read)                   analytics_delivery_rate_daily
                                               (MySQL — upsert)

              +-------------------+
              | AnalyticsRepo     |
              | SaveRollupState   |
              +---------+---------+
                        |
                        v
              analytics_rollup_state
                (MySQL — upsert)
```

Each tick:

1. Enumerate tenants via the `OrgLister` port.
2. For every tenant, call `Service.Rollup(ctx, orgID, yesterday)` and
   then `Service.Rollup(ctx, orgID, today)`.
3. Persist `today` into `analytics_rollup_state` under the
   `analytics.rollup.daily` bookmark.

## Idempotency guarantees

- Every write is `ON DUPLICATE KEY UPDATE` on the composite PK.
- Rollup is called with UTC-day granularity — same day → same rows →
  same state.
- If a tick crashes mid-tenant, the next tick catches up because it
  re-processes yesterday + today unconditionally. No per-tenant
  bookmark is required for correctness; the global bookmark is a
  diagnostic aid only.
- A tick that takes longer than the interval simply drops the extra
  tick; the following tick still produces the correct state.

## Failure isolation

- A broken tenant's Rollup error is logged and swallowed; the runner
  continues iterating.
- A broken `SaveRollupState` is logged but does not fail the tick.
- The runner never spawns unbounded goroutines: one ticker goroutine
  processes tenants sequentially.

## Reads

The `/api/v1/analytics/overview` and `/api/v1/analytics/series`
endpoints read straight from the `analytics_*_daily` tables through
`AnalyticsRepo.MessagesRange` / `ConversationsRange` /
`DeliveryRateRange`. No provider adapter is on the read path.
