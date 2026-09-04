# Revoke a token

Revocation immediately invalidates a token — subsequent bearer requests presenting it are rejected as unauthenticated. The `api_tokens` row is soft-marked (`revoked_at` set), not deleted, so historical audit + usage-log rows remain joinable.

## How to use

1. Open **Settings → API tokens** (`/settings/api-tokens`).
2. Find the token by name or prefix.
3. Click the row's overflow menu → **Revoke**.
4. Confirm.

The row stays in the list with a `Revoked` badge and its `revoked_at` timestamp. It cannot be reactivated — if you need access again, mint a new token.

## API

```
DELETE /api/v1/api-tokens/{id}
```

- Path param `id` — ULID of the `api_tokens` row.
- `204 No Content` on success.
- `404` if no matching token exists in the caller's org.
- **Idempotent** — revoking an already-revoked token succeeds.

Guarded by the `api_tokens.manage` RBAC scope.

## MCP

```json
{
  "tool": "revokeAPIToken",
  "arguments": {
    "id": "01JC5XYZTOKENID"
  }
}
```

## Soft-revocation state

- The `api_tokens` row is preserved. `revoked_at` is populated.
- Historical `api_token_usage` rows are preserved — you can still inspect what the token did before revocation. See [Usage log + metrics](/#/api-tokens/usage-log-metrics).
- The prefix + name remain visible in `listAPITokens` and audit logs so operators can trace incidents.

## Rotation pattern

To swap a token without a service outage:

1. `createAPIToken` with a new name (e.g. append `-2`).
2. Update the client config with the new plaintext.
3. Verify the client is authenticating on the new token (check `last_used_at` on the new row, or the usage log).
4. `revokeAPIToken` on the old token's `id`.

## Troubleshooting

- **Client still succeeds after revoke.** Check the client's config actually points at the revoked token — a stale env var or a cached process can keep sending an old cookie-based session instead. Grep the recent usage log for the prefix.
- **`404` on revoke.** The ULID does not exist in your org. Cross-tenant lookups are rejected at the query layer — you cannot revoke another org's token.

## Related

- [Overview](/#/api-tokens/overview)
- [Create a token](/#/api-tokens/create-token)
- [Usage log + metrics](/#/api-tokens/usage-log-metrics)
- [Audit log](/#/audit-telemetry/audit-log)
