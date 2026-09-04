-- 20260904000001_provider_calls.up.sql
-- provider_calls: operator-facing execution log for every outbound HTTP call
-- the provider adapters make to a third-party (Meta Graph API today; other
-- providers land in later phases). This is the canonical source of truth for
-- debugging "why did that send fail?" without stringing together log lines.
--
-- Design notes:
--   * BIGINT auto-increment PK — high-cardinality write path, we don't need
--     ULIDs; per-row diagnostic identity is fine as an internal sequence.
--   * org_id is VARBINARY(16) mirroring every other Phase 1 table.
--   * integration_id is NULLABLE — very-early failures (e.g. bad config
--     before the integration row is loaded) still get logged.
--   * request_body / response_body live as MEDIUMBLOB; the application layer
--     truncates at MaxBodyBytes (default 64 KiB) before persist. Media
--     download response bodies are intentionally never persisted.
--   * NEVER store Authorization headers or any secret material here.

CREATE TABLE IF NOT EXISTS provider_calls (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_id           VARBINARY(16)   NOT NULL,
  integration_id   VARBINARY(16)   NULL,
  provider         VARCHAR(32)     NOT NULL,
  operation        VARCHAR(64)     NOT NULL,
  method           VARCHAR(8)      NOT NULL,
  url              TEXT            NOT NULL,
  status_code      INT             NOT NULL DEFAULT 0,
  latency_ms       INT             NOT NULL DEFAULT 0,
  request_body     MEDIUMBLOB      NULL,
  response_body    MEDIUMBLOB      NULL,
  error_class      VARCHAR(64)     NOT NULL DEFAULT '',
  error_message    VARCHAR(1024)   NOT NULL DEFAULT '',
  trace_id         VARCHAR(128)    NOT NULL DEFAULT '',
  correlation_id   VARCHAR(64)     NOT NULL DEFAULT '',
  occurred_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_provider_calls_org_time (org_id, occurred_at),
  KEY ix_provider_calls_org_integration_time (org_id, integration_id, occurred_at),
  KEY ix_provider_calls_org_status (org_id, status_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
