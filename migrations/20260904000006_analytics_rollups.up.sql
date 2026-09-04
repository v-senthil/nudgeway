-- 20260904000006_analytics_rollups.up.sql
-- Analytics v1: daily rollup tables + rollup-worker bookkeeping row.
--
-- Design notes
--   * Rollup tables are DERIVED — they carry only aggregates. The raw
--     canonical data lives in `messages` and `conversations`. Truncating
--     any of these tables is a safe operation; the analytics rollup
--     worker re-computes them idempotently.
--   * `org_id` is VARBINARY(16), mirroring every other tenant-scoped
--     table.
--   * `day` is a DATE (UTC). The rollup worker rolls up whole UTC days —
--     no timezone conversion happens in aggregation; the read side is
--     free to render local time.
--   * The composite PRIMARY KEY on every table doubles as the upsert
--     key (ON DUPLICATE KEY UPDATE ...).
--   * Sentinel values: `provider = 'all'` and `message_type = 'all'`
--     rows carry the pan-provider / pan-type totals. Per-provider and
--     per-type detail rows live alongside them under the same
--     (org_id, day) prefix.

CREATE TABLE IF NOT EXISTS analytics_messages_daily (
  org_id        VARBINARY(16)               NOT NULL,
  day           DATE                        NOT NULL,
  provider      VARCHAR(32)                 NOT NULL DEFAULT 'all',
  direction     ENUM('inbound','outbound') NOT NULL,
  message_type  VARCHAR(32)                 NOT NULL DEFAULT 'all',
  total         INT                         NOT NULL DEFAULT 0,
  delivered     INT                         NOT NULL DEFAULT 0,
  read_count    INT                         NOT NULL DEFAULT 0,
  failed        INT                         NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, day, provider, direction, message_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS analytics_conversations_daily (
  org_id                       VARBINARY(16) NOT NULL,
  day                          DATE          NOT NULL,
  opened                       INT           NOT NULL DEFAULT 0,
  resolved                     INT           NOT NULL DEFAULT 0,
  avg_response_time_seconds    INT           NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS analytics_delivery_rate_daily (
  org_id       VARBINARY(16) NOT NULL,
  day          DATE          NOT NULL,
  provider     VARCHAR(32)   NOT NULL DEFAULT 'all',
  sent         INT           NOT NULL DEFAULT 0,
  delivered    INT           NOT NULL DEFAULT 0,
  read_count   INT           NOT NULL DEFAULT 0,
  failed       INT           NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, day, provider)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- analytics_rollup_state: single-row-per-job bookkeeping. The worker
-- updates `last_processed_day` after a successful rollup so a restart
-- resumes cleanly instead of re-computing every day since the epoch.
-- `org_id` carries a per-org bookmark when the job is sharded per-tenant;
-- the global bookmark uses the empty-string sentinel VARBINARY(16) value.
CREATE TABLE IF NOT EXISTS analytics_rollup_state (
  job_name             VARCHAR(64)   NOT NULL,
  org_id               VARBINARY(16) NOT NULL DEFAULT '',
  last_processed_day   DATE          NULL,
  last_ran_at          DATETIME(3)   NULL,
  PRIMARY KEY (job_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
