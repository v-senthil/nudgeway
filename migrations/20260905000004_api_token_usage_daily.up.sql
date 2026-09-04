-- api_token_usage_daily is the pre-aggregated rollup of api_token_usage
-- keyed by (org, token, day). Populated by the api-token usage rollup
-- worker (idempotent upsert on every tick). The read-side Metrics query
-- uses it directly so the operator UI's KPI cards stay fast even when
-- the raw log grows into millions of rows.
CREATE TABLE IF NOT EXISTS api_token_usage_daily (
  org_id          BINARY(16) NOT NULL,
  token_id        BINARY(16) NOT NULL,
  day             DATE NOT NULL,
  total_requests  INT NOT NULL DEFAULT 0,
  error_count     INT NOT NULL DEFAULT 0,
  avg_latency_ms  INT NOT NULL DEFAULT 0,
  bytes_in_total  BIGINT NOT NULL DEFAULT 0,
  bytes_out_total BIGINT NOT NULL DEFAULT 0,
  updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                    ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (org_id, token_id, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
