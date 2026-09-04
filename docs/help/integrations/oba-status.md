# Official Business Account (OBA)

Meta's Official Business Account badge — the green checkmark next to your business name in WhatsApp — unlocks features Nudgeway needs, most importantly [Groups](#/groups/overview). This section tracks the OBA application for one integration.

## Status values

| Value | Meaning |
|---|---|
| **Not applied** | No application on file. Click **Apply** to file one. |
| **Pending** | Application filed. Meta is reviewing (typically 1-5 business days). |
| **Approved** | Badge granted. Groups and other OBA-gated features work. |
| **Rejected** | Meta declined. The panel shows the reason. Fix and re-apply. |
| **Cancelled** | Application withdrawn. |

## How to use

1. Click **Settings** -> **Integrations** and pick the integration.
2. Open the **OBA** section.
3. If the status is **Not applied**, click **Apply**. Nudgeway files the application with Meta on your behalf.
4. Wait for Meta's review. The status refreshes automatically when you reopen the panel.
5. If the status is **Pending** and you want to cancel, click **Withdraw**.
6. Once **Approved**, you can go to the [Groups page](#/groups/list-sync) and click **Sync** to pull your groups.

## Troubleshooting

- **Rejected with a vague reason** — Meta doesn't always give actionable detail. Common causes are: business verification not complete, the brand name conflicts with another business, or your website is missing from the business profile. Fix and re-apply after 30 days (Meta enforces a wait window on re-applications).
- **Approved but Groups still won't sync** — Meta sometimes lags a few hours propagating OBA. Wait a day and re-try [Sync](#/groups/list-sync).
- **Apply button returns an error banner** — your WABA may be under Meta review or enforcement. Contact your Meta representative. An admin can check the exact response in the [Meta API execution log](#/audit-telemetry/provider-calls).

## Related

- [Integrations overview](#/integrations/overview)
- [Groups overview](#/groups/overview)
- [Meta API execution log](#/audit-telemetry/provider-calls)
