-- 20260905000001_analytics_calls_rollup.up.sql
-- Analytics: daily call rollup table.
--
-- Design notes
--   * DERIVED from the canonical `calls` table. Safe to truncate — the
--     analytics rollup worker re-computes it idempotently.
--   * `direction` carries the pan-direction "all" sentinel row alongside
--     the per-direction detail rows under the same (org_id, day) prefix.
--   * Composite PRIMARY KEY doubles as the upsert key.

CREATE TABLE IF NOT EXISTS analytics_calls_daily (
  org_id                  VARBINARY(16) NOT NULL,
  day                     DATE          NOT NULL,
  direction               VARCHAR(16)   NOT NULL DEFAULT 'all',
  total                   INT           NOT NULL DEFAULT 0,
  answered                INT           NOT NULL DEFAULT 0,
  completed               INT           NOT NULL DEFAULT 0,
  failed                  INT           NOT NULL DEFAULT 0,
  missed                  INT           NOT NULL DEFAULT 0,
  duration_seconds_total  INT           NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, day, direction)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
