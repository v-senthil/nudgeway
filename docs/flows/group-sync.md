# Flow: Group sync

Operator-triggered pull of the current group roster from the provider into
our own `groups` + `group_members` tables. Runs on demand from the
settings > Groups page (`POST /api/v1/groups/sync`) and can be re-run at
any time; it is idempotent thanks to the `(org_id, integration_id,
provider_group_id)` unique key on `groups` and the
`(group_id, wa_id, bsuid)` key on `group_members`.

## Sequence

```
Operator                    REST                        Application                       Provider (Meta)
   |                          |                            |                                    |
   |  POST /api/v1/groups/sync|                            |                                    |
   |------------------------->|                            |                                    |
   |                          | Service.Sync(orgID, integ) |                                    |
   |                          |--------------------------->|                                    |
   |                          |                            | Integrations.GetWithSecrets(...)   |
   |                          |                            |----+                               |
   |                          |                            |    | (loads phone_number_id +      |
   |                          |                            |    |  waba_id + access token)      |
   |                          |                            |<---+                               |
   |                          |                            | Providers.Channel("whatsapp", …)   |
   |                          |                            |----+                               |
   |                          |                            |<---+  (typed as ProviderGroupsClient)|
   |                          |                            |                                    |
   |                          |                            | ListGroups(ctx)                    |
   |                          |                            |----------------------------------->|
   |                          |                            |    GET /<pnid>/groups?limit=25     |
   |                          |                            |<-----------------------------------|
   |                          |                            |                                    |
   |                          |                            | for each summary:                  |
   |                          |                            |   GetGroup(ctx, providerGroupID)   |
   |                          |                            |----------------------------------->|
   |                          |                            |    GET /<group_id>?fields=…        |
   |                          |                            |<-----------------------------------|
   |                          |                            |   Repo.Upsert(Group)               |
   |                          |                            |   for each participant:            |
   |                          |                            |     Repo.AddMember(Member)         |
   |                          |                            |                                    |
   |                          |<---------------------------|                                    |
   |                          |  SyncResult{groups, members}                                    |
   |<-------------------------|                                                                 |
   |    200 OK                                                                                  |
```

## Failure semantics

- **Integration missing**: `Sync` returns `appgroup.ErrNoIntegration`; REST
  translates to `424 Failed Dependency`.
- **Adapter has no groups surface**: returns `appgroup.ErrUnsupported`;
  REST translates to `422 Unprocessable Entity`. This is the case for a
  non-WhatsApp channel today.
- **Provider transport failure on `ListGroups`**: bubbles up as a 502
  `provider_error`. The operator can retry safely — nothing has been
  persisted yet.
- **Per-group failure** (rare — one group id in the returned page is stale
  or 4xx-s on `GetGroup`): logged at WARN and skipped. The rest of the page
  proceeds; `SyncResult.GroupsUpserted` reports only the successful ones.

## Idempotency

- Same `(org_id, integration_id, provider_group_id)` triple re-runs update
  the existing row's `subject`, `description`, `size`, `is_admin`, and
  `metadata`. The `id` and `created_at` stay stable.
- Members are keyed on `(group_id, wa_id, bsuid)`; re-runs update the role
  and re-open the row if `left_at` was previously set (rejoin case).
- Sync does not tombstone members Meta no longer returns. A future
  reconciliation pass will diff the returned set against the persisted
  set and stamp `left_at` on rows Meta dropped; that pass will be its own
  flow doc.

## Observability

Every outbound HTTP call fires a `TraceEvent` via the WhatsApp adapter's
tracer, landing in `provider_calls` with operation tags `list_groups` +
`get_group`. Filter `GET /api/v1/provider-calls?operation=list_groups` in
the operator UI to see the run history.

## Code references

- REST: `internal/api/rest/v1/groups.go` — `groupsHandler.sync`
- Application: `internal/application/group/service.go` — `Service.Sync`
- Adapter: `internal/providers/whatsapp/groups.go` — `ListGroups`,
  `GetGroup`
- Reference: `~/Documents/whatsapp_doc_tracker/docs/groups/reference.md`
