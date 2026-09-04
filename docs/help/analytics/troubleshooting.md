# Analytics troubleshooting

## Nudgeway tab: all cards show 0

**What you see**: Every KPI on the Nudgeway tab reads `0` or `-`. Sparklines are flat.

**Most common cause**: The 15-minute rollup hasn't fired yet since you sent your first message.

**What to do**:

- Wait 15 minutes and refresh.
- Confirm the range picker isn't set to a future date.
- Confirm you have actually sent or received messages in the selected range — open the [Inbox](#/inbox/overview) as a sanity check.

## Nudgeway tab: sparkline stays flat at "max 2"

**What you see**: The chart's Y-axis maxes at 2 and the line is flat along the bottom.

**Why**: The chart uses a display floor when there is no activity, so an empty range shows as "max 2" rather than a collapsed axis.

**What to do**: Send some real traffic, wait for the next 15-minute tick, and refresh.

## Meta Analytics: pricing tier column is empty

**What you see**: The Pricing sub-section shows rows but the tier column is blank on some of them.

**Why**: Meta omits the tier for free-tier, free-entry-point, and free-customer-service pricing types. Not an error — those categories aren't billed by tier.

**What to do**: Nothing to fix. If you want tier-bearing rows only, add `REGULAR` to the pricing-types filter.

## Meta Analytics: red error banner from Meta

**What you see**: A sub-section shows a red banner with a Meta message like "The parameter phone_numbers is required".

**What to do**:

- Wait a couple of seconds and refresh — the integration picker may still be loading the phone number.
- Type your WhatsApp number in E.164 format (no `+`) into the phone-numbers box.
- Widen the range. Some Meta endpoints reject very short ranges outside the half-hour granularity.

## Meta Analytics: integration picker won't switch

**What you see**: You choose a different integration in the drop-down, but the sub-sections keep showing the old numbers.

**What to do**:

- Change the range in the range picker to force a refresh.
- Do a hard-refresh (Cmd+Shift+R on macOS, Ctrl+Shift+R on Windows / Linux).
- If a panel is stuck on a failed load, the tab may be showing the last successful data. Reload the page.

## Either tab: "Unauthorized" or the page redirects to sign-in

**What you see**: The Analytics page prompts you to sign in, or a red banner mentions "Unauthorized".

**What to do**: Your session has expired. Sign in again.

## Either tab: "Missing permission" banner

**What you see**: A red banner mentions the `analytics.read` permission.

**What to do**: Ask an org admin to grant your role analytics-read access.

## Related

- [Analytics overview](#/analytics/overview)
- [Nudgeway tab](#/analytics/nudgeway-tab)
- [Meta Analytics tab](#/analytics/meta-analytics-tab)
- [Meta API execution log](#/audit-telemetry/provider-calls)
