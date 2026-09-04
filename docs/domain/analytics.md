# Domain — Analytics

The `analytics` domain package models the derived, provider-agnostic
aggregates the operator dashboard renders on top of the canonical
`messages` and `conversations` tables. It is **derived state** — every
row can be re-computed from raw data at any time.

## Purpose

Operators need at-a-glance answers to:

- How many messages did we exchange today / this week?
- What is our delivery rate, broken down by provider?
- How many conversations opened yesterday, and how fast did we reply?

Answering these live off the raw `messages` + `conversations` tables
would either be too slow (10s of thousands of rows scanned per card
render) or too weak (no per-provider comparison because JOINs across
`integrations` would be required). The rollup pipeline pre-aggregates
the counts once per (org, day) and the dashboard reads from
`analytics_*_daily` tables straight.

## Types

- `analytics.MessagesDaily` — one row per (org, day, provider,
  direction, message_type). The pan-provider aggregate lives under
  provider="all"; the pan-type aggregate lives under message_type="all".
  A single (all, all) row per direction carries the grand totals.
- `analytics.ConversationsDaily` — one row per (org, day) with opened +
  resolved counters and a coarse response-time average.
- `analytics.DeliveryRateDaily` — one row per (org, day, provider) with
  sent / delivered / read / failed counters on outbound traffic.
- `analytics.Overview` — the compact aggregate the dashboard cards
  render.
- `analytics.Series` — a labelled list of `(day, value)` points a chart
  can plot.

## Rollup semantics

- Rollup runs once every 15 minutes for every enumerated tenant.
- Each pass rolls up **yesterday and today** — the double-run catches
  late-arriving webhook updates that mutate a message's status after
  midnight UTC (e.g. a `read` receipt on a message sent at 23:58 the
  previous day).
- Rollup is **idempotent**: every write is an upsert on the composite
  primary key, so running it twice for the same day produces the same
  table state. A restart mid-flight is safe.
- Rollup is scoped to whole **UTC days**. Rendering local time is a
  read-side concern.

## Backfill

The rollup worker persists `last_processed_day` in
`analytics_rollup_state` after every tick. On a fresh install, the
first tick rolls up yesterday + today only. To backfill historical
days, run the admin CLI subcommand `nudgeway analytics rollup --from
YYYY-MM-DD --to YYYY-MM-DD` (planned) which drives `Service.Rollup` in
a loop. Backfill is safe to interrupt because every day is idempotent.

## Provider-agnosticity

The `analytics` package imports zero provider SDKs. Provider names
appear only as opaque strings in the `provider` column — the same code
path handles WhatsApp today and Twilio / Zoho Desk tomorrow with no
domain changes.
