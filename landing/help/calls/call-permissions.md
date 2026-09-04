# Call permissions

WhatsApp users control which businesses can call them. Before you place an outbound call, you must check the recipient's current permission and — if they haven't granted one — send an interactive `call_permission_request` message and wait for them to tap Accept.

## The three states

| State | Meaning | Can I call? |
|---|---|---|
| `permanent` | User granted permission indefinitely | Yes, freely |
| `temporary` | User granted a time-limited permission | Yes, before `expiration_time` |
| `no_permission` | Never granted, or a previous grant lapsed | No — send a permission-request first |

## Check the current state

**operationId**: `getIntegrationCallPermission`

```
GET /api/v1/integrations/{id}/call-permission?to=<E164 without +>
```

Requires `calls.read` (a lower bar than `integrations.manage`).

```bash
curl -sS 'http://127.0.0.1:8080/api/v1/integrations/01M1…/call-permission?to=918197002143' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>'
```

Response (`CallPermission`):

```json
{ "status": "temporary", "expiration_time": 1793212800 }
```

- `expiration_time` is Unix seconds. Zero when `status` is `permanent` or `no_permission`.
- The "New call" affordance in the UI uses this exact call to render a permanent/temporary/no-permission chip and enable/disable the Call button.

## Request permission

When `status: no_permission`, send an interactive `call_permission_request` message. The user sees a WhatsApp-native prompt with Accept / Decline; their reply arrives as an inbound `call_permission_reply` bubble in the thread.

**operationId**: `sendCallPermissionRequest`

```
POST /api/v1/calls/permission-request
```

```bash
curl -sS -X POST 'http://127.0.0.1:8080/api/v1/calls/permission-request' \
  -H 'Authorization: Bearer nk_abcd1234_<40-char-secret>' \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "01M1MC4KFJQ33YQWKPT7HKZNYC",
    "to": "918197002143",
    "prompt": "May we call you to discuss your recent order?"
  }'
```

Response (`CallPermissionRequestResponse`):

```json
{ "wamid": "wamid.HBg…" }
```

The `wamid` is the provider-issued id of the outbound interactive message.

The `postCallPermissionRequested` audit action fires whenever a permission-request message is sent (visible in the [Audit log](/#/audit-telemetry/audit-log)).

## MCP

- `getIntegrationCallPermission` — `{ "id": "<integration-ULID>", "to": "<E164>" }`
- `sendCallPermissionRequest` — `{ "body": { "integration_id": "...", "to": "...", "prompt": "..." } }`

## Recipient's reply

The customer's `accept` / `reject` reply arrives as a `call_permission_reply` message inbound. Nudgeway renders it as a distinct bubble in the thread; the next `getIntegrationCallPermission` call will show the updated state.

## Troubleshooting

- **`400 missing to parameter`** — pass `?to=<E.164 without +>` on the GET.
- **`403 missing calls.read`** — role doesn't grant call reads. Admin can grant.
- **`404 integration not found`** — the ULID is wrong or the integration was deleted.
- **`501 call-permission lookup is not wired for this deployment`** — the WhatsApp adapter version in this build doesn't support the permission endpoint. Upgrade Nudgeway.
- **`502 provider error`** — Meta returned a transient error. Retry; the [Provider calls log](/#/audit-telemetry/provider-calls) has the exact response.
- **User tapped Accept but state still `no_permission`** — the reply webhook is delayed. Wait 30-60s and re-fetch; if still stuck, check webhook subscriptions include `call_settings_update`.

## Related

- [Outbound calls](/#/calls/outbound-call) — placing the call once permission exists.
- [Calls overview](/#/calls/overview) — full call lifecycle.
- [Call settings](/#/integrations/call-settings) — per-integration call hours and defaults.
