# Groups

A **Group** is a persisted, tenant-scoped mirror of a WhatsApp Business Groups thread. Nudgeway treats groups as first-class alongside `Contact`, `Session`, `Conversation`, `Message`, and `Ticket` — a group conversation shows up as **one thread in the Inbox** regardless of how many participants have written to it.

## OBA-only

The WhatsApp Business Cloud Groups API is gated by Meta on **Official Business Account (OBA)** phone numbers. If your integration has not been granted OBA:

- `POST /api/v1/groups/sync` returns `502 provider_error` from Meta.
- The Groups page in Settings renders empty.

Check your OBA state at [Settings → Integrations → OBA](/#/integrations/oba-status). Apply once, wait for `APPROVED`, then re-run sync.

## Model

Every group row carries:

- `provider_group_id` — the Meta group id.
- `subject` (may be blank) + optional `description`.
- `size` — hint from Meta's `total_participant_count`; the authoritative roster count comes from `group_members` filtered by `left_at IS NULL`.
- `is_admin` — whether *our* business phone number holds admin rights. Management calls (add/remove participants, reset invite link) require `is_admin = true`; non-admin groups are read-only.
- `metadata` — free-form bag for provider-native fields (`join_approval_mode`, `suspended`, `creation_timestamp`).

Members are keyed on `(group_id, wa_id, bsuid)`. A member row can exist before the participant has ever messaged you (so `contact_id` may be nil until an inbound message resolves them).

## Inbox behaviour

A group message webhook still fires the canonical `MessageReceived` event with `from` = the participant's contact identity. The `group_id` rides as a sidecar so the inbox fan-out routes it to the group thread rather than a 1:1 chat.

## Related

- [List + sync groups](/#/groups/list-sync)
- [Send to a group](/#/groups/send-to-group)
- [Groups troubleshooting](/#/groups/troubleshooting)
- [OBA status](/#/integrations/oba-status)
- Source of truth: `docs/domain/group.md`, `docs/flows/group-sync.md`.
