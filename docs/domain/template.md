# Template

**Package**: `internal/domain/template`
**Code**: [`template.go`](../../internal/domain/template/template.go), [`errors.go`](../../internal/domain/template/errors.go)
**Table**: `templates` (see [`migrations/20260904000003_templates.up.sql`](../../migrations/20260904000003_templates.up.sql))
**Port**: [`internal/ports/repository/templates.go`](../../internal/ports/repository/templates.go)
**Application service**: [`internal/application/template/service.go`](../../internal/application/template/service.go)
**REST handler**: [`internal/api/rest/v1/templates.go`](../../internal/api/rest/v1/templates.go)
**Provider adapter**: [`internal/providers/whatsapp/templates.go`](../../internal/providers/whatsapp/templates.go)

## Purpose

A `Template` is a tenant-scoped, provider-neutral message template — the
canonical name/language/category/components record that lets an operator
draft, submit, sync, and (eventually) send templated messages through any
channel provider.

Templates today are sourced from the WhatsApp Business Cloud API
`/message_templates` endpoint. The domain vocabulary intentionally
mirrors Meta's (categories `MARKETING` / `UTILITY` / `AUTHENTICATION`,
statuses `DRAFT` / `PENDING` / `APPROVED` / `REJECTED` / `PAUSED` /
`DISABLED`) because Meta covers 100% of the concept space we've mapped so
far. Other providers map their native shape onto this canonical model at
the adapter boundary.

## Invariants

- Every row is tenant-scoped. Every query includes `org_id` — infra
  enforces this at the SQL layer via composite indexes.
- The `(org_id, integration_id, name, language)` tuple is unique.
  Provider adapters enforce the same uniqueness on their side; the local
  mirror follows.
- `Status = DRAFT` is a fullWA-only concept. A DRAFT row has never been
  sent to the provider and can be freely edited or deleted. Every other
  status is a mirror of the provider's review state.
- `Components` is required to include at least one `BODY` component.
  Meta rejects submissions without one; we reject them at the REST edge
  so a DRAFT round-trip does not hit a provider-side 400.
- `ProviderTemplateID` is set once, on the transition
  `DRAFT → PENDING`. Once set, it never changes.
- `LastSyncedAt` is stamped whenever the row was reconciled with the
  provider — after a successful `SubmitForReview` or `Sync` round-trip.
  NULL means "never synced" (fresh DRAFT).

## Lifecycle

```
[operator draft] ─ Create ─► DRAFT ─ SubmitForReview ─► PENDING ─► APPROVED
                                                              └─► REJECTED
                                                              └─► PAUSED  ─► APPROVED  (Meta re-issues)
                                                              └─► DISABLED (terminal)
```

`Sync` reconciles the mirror with the provider without an explicit
transition — a Sync call may flip PENDING → APPROVED after Meta finishes
review.

## Related entities

- [`Integration`](integration.md) — a `Template` always belongs to one
  Integration (the WABA / phone number pair on Meta side). Deleting an
  integration orphans templates; Phase 3 will cascade this cleanup.
- `Message` — outbound sends of type `template` reference a Template by
  `name + language`. Wire-up between Message.Send and Template lookup is
  Phase 3.

## Testing notes

- Domain tests live next to the domain file — pure Go, no infra.
- Integration tests hit real MySQL (see the `_integration_test.go`
  neighbours of similar repositories in `internal/infrastructure/mysql`).
- Provider tests use `httptest` fixtures under
  `internal/providers/whatsapp/testdata/` (Phase 2 Task 5 lands the
  fixture set).
