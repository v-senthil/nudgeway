# OpenAPI CHANGELOG

Every change to `internal/api/openapi/openapi.yaml` gets an entry here.

## 0.2.4-phase2 — 2026-09-04

- Added `GET /api/v1/audit-logs` — paginated read of the tenant audit trail. Auth + `audit.read`. Query params: `resource_type`, `resource_id`, `action`, `actor_user_id`, `since` (RFC3339), `until` (RFC3339), `cursor`, `limit` (1..200, default 50). Response is `{items, next_cursor}` newest-first.
- Added schemas `AuditLog`, `AuditLogList`, `AuditLogFilter`.
- Added `audit` OpenAPI tag.
- Added `GET /api/v1/provider-calls` — operator-facing execution log for every outbound HTTP call the provider adapters made (WhatsApp `send_message`, `mark_as_read`, `get_media_url`, `download_media`, `list_templates`, `create_template`, `get_template_status`, `upload_media`). Auth + `integrations.manage`. Query params: `integration_id`, `operation`, `status_min`, `status_max`, `since`, `until`, `cursor`, `limit` (1..200, default 50). Response is `{items, next_cursor}` newest-first, with base64-encoded request / response bodies (truncated at 64 KiB; empty for `download_media` response body).
- Added schemas `ProviderCall`, `ProviderCallList`.
- Added `provider-calls` OpenAPI tag.

## 0.2.2-phase1 — 2026-09-04

- Added `GET /api/v1/media/{key}` — streams a stored media blob keyed by SHA-256 hex. Auth-gated (`sessionCookie`), response served as `application/octet-stream` with the persisted `Content-Type`, `Cache-Control: private, max-age=86400`. Errors: `401`, `404` (unknown key).
- Added `HEAD /api/v1/media/{key}` — headers-only companion for prefetch / existence checks.
- Extended `Message` schema with `text` (body / caption), `media_url` (same-origin `/api/v1/media/{key}` for downloaded inbound media, provider-native URL as fallback), and `content_type`.

## 0.2.1-phase1 — 2026-09-04

- Added `GET /api/v1/integrations` — list every integration for the current org (`integrations.manage`).
- Added `POST /api/v1/integrations` — create + envelope-encrypt secrets; also upserts a `business_endpoints` row for channel-kind providers. Auth + CSRF + `integrations.manage`.
- Added `GET /api/v1/integrations/{id}` — fetch one; secrets are stripped.
- Added `POST /api/v1/integrations/{id}/test` — resolves the provider adapter, runs `HealthCheck`, updates `Status` + `Health`. Auth + CSRF + `integrations.manage`.
- Added `DELETE /api/v1/integrations/{id}` — soft-disconnects (row remains for the Phase 4 audit trail). Auth + CSRF + `integrations.manage`.
- Added `Integration`, `IntegrationList`, `CreateIntegrationRequest`, `TestIntegrationResponse` schemas.

## 0.2.0-phase1 — 2026-09-04

- Added `GET /webhooks/{provider}/{integration_id}` — Meta subscription verification handshake. Unauthenticated (`security: []`). Query params `hub.mode`, `hub.verify_token`, `hub.challenge`. Returns 200 `text/plain` with the challenge on match, 403 problem+json on mismatch.
- Added `POST /webhooks/{provider}/{integration_id}` — provider webhook ingress. Unauthenticated at the HTTP layer (`security: []`); authenticity is enforced per-provider by signature verification inside the ingress helper. Returns 200 with an empty body on accepted or duplicate delivery, 401 problem+json on signature failure, 413 problem+json on bodies larger than 1 MiB.
- Added `POST /api/v1/messages` — enqueues an outbound send. Auth + CSRF. Returns `202 {message_id, status:"queued"}`. Errors: `400 validation` (bad payload), `404 conversation_not_found`, `424 integration_missing` when the conversation's endpoint has no configured integration.
- Added `GET /api/v1/conversations/{id}/messages` — cursor-paginated message list for a conversation, newest first. Query params `cursor`, `limit` (1..200, default 50). Auth.
- Added `GET /api/v1/conversations` — Phase 1 placeholder empty list; the full impl lands under Phase 1 Task 4. Auth.
- Added schemas: `SendMessageRequest`, `SendMessageAccepted`, `Message`, `MessageList`, `Conversation`, `ConversationList`.

## 0.1.1 — 2026-09-04

- **`Me` schema** now includes required `email` and `display_name`. Fixes a frontend inbox crash where the initials helper called `.trim()` on undefined. (`3bc7132`)

## 0.1.0-phase0-auth — 2026-09-03

- Added `GET /api/v1/auth/csrf` — issues the double-submit CSRF cookie for the first login.
- Added `POST /api/v1/auth/login` — email + password login; sets session + CSRF cookies.
- Added `POST /api/v1/auth/logout` — invalidates the session; clears cookies. Requires CSRF.
- Added `GET /api/v1/auth/me` — returns the current principal, org, and permissions.
- Added `LoginRequest`, `LoginResponse`, `Me` schemas.

## 0.1.0-phase0 — 2026-09-03

- Initial spec.
- Added `GET /healthz` — liveness probe.
- Added `GET /readyz` — readiness probe. Returns 503 when downstream deps unreachable (probes land Phase 0 Task 2).
- Defined `Problem` schema (RFC 7807) as the canonical error body.
- Defined `sessionCookie` + `apiKey` security schemes.
