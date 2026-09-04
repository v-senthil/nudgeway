# Business username

The Business-Scoped Username (BSUID handle) is a `@handle` a customer can use to find the business. Adopting one lets you send messages using the username instead of the phone number (see the [BSUID send-order](/#/inbox/send-text) preference).

Empty username is a **valid steady state** — you can operate without one.

## How to use

1. Settings → Integrations → the row → **Username** section.
2. Click **Suggestions** — Nudgeway asks Meta for candidate handles based on your verified name.
3. Pick one or type your own. Save.
4. If your desired handle is owned by another business, choose a `transfer_action` (see below) and re-save.
5. To release the handle, click **Delete**.

## Transfer semantics

`SetUsernameRequest.transfer_action` controls conflict resolution when another business owns the handle:

| Value | Behaviour |
|---|---|
| `none` (default) | Fail with a conflict error if the handle is taken. |
| `force_transfer` | Take ownership away from the current owner. Only works when Meta considers you the rightful owner (trademark, prior claim, ...). Meta rejects with an error otherwise. |

`force_transfer` is destructive to the losing business. Nudgeway does not verify entitlement — Meta does. If Meta accepts, the previous owner's handle is gone.

## API

### Read current username

```
GET /api/v1/integrations/{id}/username
```

Response `200`:

```json
{ "username": "acmesupport", "status": "approved" }
```

Empty `username` = no handle adopted. `status` is `"approved" | "reserved" | ""`.

### Set / change

```
PUT /api/v1/integrations/{id}/username
Content-Type: application/json

{
  "username": "acmesupport",
  "transfer_action": "none"
}
```

Response `200` — the adopted username. Requires CSRF + `integrations.manage`.

### Delete (release)

```
DELETE /api/v1/integrations/{id}/username
```

Response `200`:

```json
{ "success": true }
```

Requires CSRF + `integrations.manage`.

### Suggestions

```
GET /api/v1/integrations/{id}/username/suggestions
```

Response `200`:

```json
{ "suggestions": ["acmesupport", "acmehelp", "askacme"] }
```

May be empty. Requires `integrations.manage`.

## MCP

| operationId | Purpose |
|---|---|
| `getIntegrationUsername` | Read the current username. |
| `setIntegrationUsername` | Adopt or change. |
| `deleteIntegrationUsername` | Release the handle. |
| `getIntegrationUsernameSuggestions` | Fetch provider-generated candidates. |

## Troubleshooting

- **`400 missing username`** — the body's `username` field is empty. Use `DELETE` to release, not `PUT` with `""`.
- **`502 conflict`** — someone else owns the handle. Try `force_transfer` only if you're entitled (Meta will verify).
- **Suggestions list is empty** — Meta had no viable candidates. Type your own.

## Related

- [Integrations overview](/#/integrations/overview)
- [Business profile](/#/integrations/business-profile)
- [Phone number details + QR](/#/integrations/phone-number-details)
