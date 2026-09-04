-- api_token_usage is the per-request execution log for every bearer-
-- authenticated call served by /api/v1/*. It mirrors the shape of
-- provider_calls (outbound provider telemetry) but records inbound
-- API traffic keyed by the api_tokens row that authenticated it.
--
-- Request / response bodies land as capped MEDIUMBLOBs (see the
-- application-service truncation in internal/application/apitokenusage);
-- true wire sizes live in the dedicated bytes columns so metrics stay
-- accurate even when bodies are clipped for storage.
CREATE TABLE IF NOT EXISTS api_token_usage (
  id             BINARY(16) NOT NULL,
  org_id         BINARY(16) NOT NULL,
  token_id       BINARY(16) NOT NULL,
  occurred_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  request_id     VARCHAR(64) NOT NULL,
  method         VARCHAR(10) NOT NULL,
  path           VARCHAR(500) NOT NULL,
  status_code    SMALLINT NOT NULL,
  latency_ms     INT NOT NULL,
  remote_ip      VARCHAR(45) NOT NULL,
  user_agent     VARCHAR(500) NULL,
  request_body   MEDIUMBLOB NULL,
  response_body  MEDIUMBLOB NULL,
  request_bytes  INT NOT NULL DEFAULT 0,
  response_bytes INT NOT NULL DEFAULT 0,
  error_message  VARCHAR(1000) NULL,
  PRIMARY KEY (id),
  KEY idx_token_time (token_id, occurred_at DESC),
  KEY idx_org_time   (org_id, occurred_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
