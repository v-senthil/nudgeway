# Audit record flow

`application/audit.Service.Record` is the single ingress for every audit trail write. It is called from every mutation path in the application layer — but the call must never break the caller's request.

## Shape

```
mutation handler ─▶ application/*.Service ─▶ commit tx
                                    │
                                    └▶ audit.Service.Record(ctx, Entry)
                                              │  (fire-and-forget, logs on error)
                                              ▼
                                    repository.AuditRepo.Record
                                              │
                                              ▼
                                    MySQL audit_logs INSERT
```

## Why fire-and-forget

`Service.Record` returns no error. This is deliberate:

1. **Correctness over completeness.** If MySQL rejects an audit insert, the user's mutation has already committed — returning the error to the caller would surface an internal failure on a successful request. Worse, if the caller then rolls back its own tx, the observable state diverges from the audit trail.
2. **Availability.** A degraded audit table (index rebuild, replica lag) would take down every mutation path. The trail can lag; the platform cannot.
3. **Legal hold escape hatch.** Callers that need a hard guarantee (e.g. a compliance export) go straight to `repository.AuditRepo.Record` and handle the error themselves. This is the exception, not the rule.

Failures are logged at `WARN` level with `org_id`, `action`, `resource_type`, `resource_id`, and the underlying error so operations can alert on sustained miss rates.

## Callers (planned wire-up)

The following mutation services will call `audit.Service.Record` once the wire-up commit lands:

| Package | Entry point | Action |
|---|---|---|
| `internal/application/integration` | `Service.Create` | `integration.created` |
| `internal/application/integration` | `Service.Delete` | `integration.deleted` |
| `internal/application/integration` | `Service.Test` | `integration.tested` |
| `internal/application/message` | `SendService.RequestSend` | `message.sent` |
| `internal/application/message` | `ReadService.MarkRead` | `message.marked_read` |
| `internal/application/message` | `ReadService.MarkConversationRead` | `conversation.marked_read` |
| `internal/api/rest/v1` (attachments) | `attachmentsHandler.upload` | `attachment.uploaded` |
| `internal/application/auth` | `SessionService.Login` | `user.logged_in` |
| `internal/application/auth` | `SessionService.Logout` | `user.logged_out` |

Each caller populates:

- `OrgID` from the authenticated principal.
- `ActorUserID` from the principal (or nil for system-driven flows).
- `Action` — the constant above.
- `ResourceType` + `ResourceID` — the affected entity.
- `IP` — from the request context (via a middleware that stashes `net.IP` on the context).
- `Metadata` — structured context (e.g. `{"provider": "whatsapp", "conversation_id": "…"}` on `message.sent`).

## Retention

Not enforced yet. Phase 4 ships a scheduled job that deletes rows with `occurred_at < now - retention_window` per org. Until then, rows accumulate and are pruned manually if needed.

## Read side

`GET /api/v1/audit-logs` (documented in [`docs/domain/audit.md`](../domain/audit.md)) is the sole read path.
