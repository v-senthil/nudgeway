-- api_tokens holds long-lived programmatic-access tokens that authenticate
-- non-browser callers (MCP server, CI, custom scripts). The plaintext token
-- (nk_<prefix>_<secret>) is returned to the caller exactly once at creation
-- time; only the prefix (indexed, shown in the UI) and an argon2id hash of
-- the secret are persisted.
CREATE TABLE IF NOT EXISTS api_tokens (
  id            BINARY(16) NOT NULL,
  org_id        BINARY(16) NOT NULL,
  user_id       BINARY(16) NOT NULL,
  name          VARCHAR(120) NOT NULL,
  prefix        CHAR(8) NOT NULL,
  secret_hash   VARBINARY(255) NOT NULL,
  scopes        JSON NULL,
  last_used_at  DATETIME(3) NULL,
  expires_at    DATETIME(3) NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at    DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_api_tokens_prefix (prefix),
  KEY idx_api_tokens_org (org_id, revoked_at, created_at DESC),
  KEY idx_api_tokens_user (user_id, revoked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
