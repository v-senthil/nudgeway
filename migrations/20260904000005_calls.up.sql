-- 20260904000005_calls.up.sql
-- calls: canonical call log for WhatsApp Business Calling API and any future
-- voice provider. One row per call — inbound (user-initiated) or outbound
-- (business-initiated) — with the full state machine (queued → ringing →
-- answered → in_progress → completed | missed | failed | declined | no_answer).
--
-- Design notes:
--   * VARBINARY(16) ULID PK — matches every other Phase 1+ table.
--   * org_id is the leading column of every non-PK index (tenancy invariant).
--   * provider + provider_call_id form the idempotency key for webhook
--     ingest (Meta's wacid.* is unique per WhatsApp Business Account).
--   * business_endpoint_id + contact_id + session_id + conversation_id are
--     nullable so we can persist a row even before we've resolved every
--     linkage — the webhook consumer backfills as it goes.
--   * from_user_id / to_user_id capture BSUIDs (Meta's <CC>.<alnum> shape).
--   * recording_url is a TEXT column pointing at either the Meta short-lived
--     URL, or (after the Phase 4 downloader lands) an internal /api/v1/media
--     key. transcription_ref is the media asset ID for the transcript JSON.
--   * metadata is a JSON column holding provider-specific fields (SDP hash,
--     biz_opaque_callback_data, error codes, etc.).

CREATE TABLE IF NOT EXISTS calls (
  id                    VARBINARY(16)  NOT NULL,
  org_id                VARBINARY(16)  NOT NULL,
  integration_id        VARBINARY(16)  NOT NULL,
  business_endpoint_id  VARBINARY(16)  NULL,
  contact_id            VARBINARY(16)  NULL,
  session_id            VARBINARY(16)  NULL,
  conversation_id       VARBINARY(16)  NULL,
  provider              VARCHAR(32)    NOT NULL,
  provider_call_id      VARCHAR(64)    NOT NULL,
  direction             ENUM('inbound','outbound') NOT NULL,
  status                VARCHAR(32)    NOT NULL,
  from_number           VARCHAR(32)    NOT NULL DEFAULT '',
  to_number             VARCHAR(32)    NOT NULL DEFAULT '',
  from_user_id          VARCHAR(160)   NOT NULL DEFAULT '',
  to_user_id            VARCHAR(160)   NOT NULL DEFAULT '',
  started_at            DATETIME(3)    NULL,
  answered_at           DATETIME(3)    NULL,
  ended_at              DATETIME(3)    NULL,
  duration_seconds      INT            NOT NULL DEFAULT 0,
  hangup_reason         VARCHAR(64)    NOT NULL DEFAULT '',
  recording_url         TEXT           NULL,
  transcription_ref     VARCHAR(64)    NOT NULL DEFAULT '',
  metadata              JSON           NOT NULL,
  created_at            DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at            DATETIME(3)    NULL ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_calls_org_provider_ext (org_id, provider, provider_call_id),
  KEY ix_calls_org_created (org_id, created_at),
  KEY ix_calls_org_status (org_id, status),
  KEY ix_calls_org_contact_created (org_id, contact_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
