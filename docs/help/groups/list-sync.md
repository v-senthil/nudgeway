# List + sync groups

Two operations back the Settings → Groups page: `listGroups` (read local rows) and `syncGroups` (pull the current roster from Meta and upsert). Sync is idempotent — the `(org_id, integration_id, provider_group_id)` unique key means re-running only refreshes existing rows.

## How to use

1. **Settings → Groups** — the page lists every persisted group scoped to the caller's org.
2. Filter by integration (drop-down) or subject substring (search box).
3. Click **Sync** on an integration to pull the current group roster from Meta into `groups` + `group_members`.

Sync loads `phone_number_id`, `waba_id`, and the encrypted access token from the integration; the adapter calls `GET /<pnid>/groups?limit=25` and then `GET /<group_id>?fields=…` per summary row. Every HTTP round-trip is recorded in [Meta API execution log](/#/audit-telemetry/provider-calls) under `operation=list_groups` and `operation=get_group`.

## API

### List

```
GET /api/v1/groups?integration_id=<ULID>&q=<subject>&cursor=<opaque>&limit=50
```

Response `200`:

```json
{
  "items": [
    {
      "id": "01J…",
      "org_id": "01J…",
      "integration_id": "01J…",
      "provider_group_id": "120363xxxxx@g.us",
      "subject": "Acme launch team",
      "size": 34,
      "is_admin": true,
      "created_at": "2026-09-01T10:12:03Z",
      "updated_at": "2026-09-04T08:44:11Z"
    }
  ],
  "next_cursor": "eyJ…"
}
```

Requires `groups.read`.

### Sync

```
POST /api/v1/groups/sync
Content-Type: application/json

{ "integration_id": "01J…" }
```

Response `200`:

```json
{ "groups_upserted": 12, "members_upserted": 387 }
```

Requires CSRF + `groups.manage`. Additional statuses: `422` (adapter has no groups surface — non-WhatsApp channel), `424` (integration missing), `502` (Meta transport failure — safe to retry, nothing was persisted).

### Roster

```
GET /api/v1/groups/{id}/members
```

Returns every `group_members` row for the group. `wa_id` is E.164 without `+`; `role` is one of `member | admin | superadmin`.

## MCP

| operationId | Purpose |
|---|---|
| `listGroups` | List persisted groups (org-scoped, cursor-paginated). |
| `syncGroups` | Pull the current roster from Meta and upsert. Idempotent. |
| `getGroup` | Read one group by ULID. |
| `listGroupMembers` | Read the roster of one group. |

## Troubleshooting

- **`groups_upserted: 0`** — the integration has no OBA. See [Groups troubleshooting](/#/groups/troubleshooting).
- **Members disappearing** — sync does not tombstone members Meta drops today; a future reconciliation pass will stamp `left_at`. Rejoin re-opens the row.
- **Stale roster** — re-run sync; the adapter always fetches the full page.

## Related

- [Groups overview](/#/groups/overview)
- [Send to a group](/#/groups/send-to-group)
- [Meta API execution log](/#/audit-telemetry/provider-calls)
