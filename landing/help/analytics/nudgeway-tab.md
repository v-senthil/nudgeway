# Nudgeway tab (local KPIs)

The Nudgeway tab shows KPIs drawn from your own send / receive activity, rolled up every 15 minutes. Reads are instant — no third-party dependency, no waiting for Meta.

## KPI cards

| Card | What it measures |
|---|---|
| **Messages total** | Every message sent or received in the selected range, across all your integrations. |
| **Delivery rate** | Delivered messages divided by sent messages, as a percentage from 0 to 100. Shows zero if no messages were sent. |
| **Response time p50** | The typical time between an inbound customer message and your next outbound reply, over the range. |
| **Conversations opened** | How many new conversations began in the range. |
| **Calls total** | Every call in the range, in either direction. |
| **Calls answered** | The subset that were picked up. |
| **Avg call duration** | Weighted average across answered calls only. |

Each KPI has a matching sparkline underneath — one point per day in the range.

## How to use

1. Click **Analytics** in the top nav. The **Nudgeway** tab is selected by default.
2. Use the range picker in the top-right to choose the window. Default is the last 14 days ending today.
3. The cards and sparklines refresh automatically when you change the range.

## Update cadence

A background job runs every 15 minutes and refreshes the rollups for yesterday, today, and tomorrow (so timezone edges settle correctly). If you just sent your first message, wait for one tick before the numbers appear. Rollups never lose data — a delayed tick simply catches up on the next run.

## Troubleshooting

- **All cards show 0 or a dash** — the rollup has not fired yet since your first message. Wait 15 minutes and refresh. If the cards are still all zero after that, see [Analytics troubleshooting](#/analytics/troubleshooting).
- **Sparkline sits flat at "max 2"** — that is the display floor when there is no activity. Send some real messages and wait 15 minutes.
- **The range picker is set to a future date** — no data can exist there yet. Reset to the default.

## Related

- [Analytics overview](#/analytics/overview)
- [Meta Analytics tab](#/analytics/meta-analytics-tab)
- [Analytics troubleshooting](#/analytics/troubleshooting)
