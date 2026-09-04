-- Reverse: narrow back to VARCHAR(64). Only safe if no live rows exceed
-- 64 chars — the down migration truncates on the storage layer error out.
ALTER TABLE calls
  MODIFY provider_call_id VARCHAR(64) NOT NULL;
