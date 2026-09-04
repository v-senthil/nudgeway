-- 20260904000011_conversations_type.down.sql
--
-- WARNING: This partial rollback drops the type + group_id columns and their
-- indexes. It does NOT restore session_id / contact_id to NOT NULL — group
-- conversations may have written NULLs into those columns, so re-adding the
-- NOT NULL constraint would fail. If a full rollback is required, an
-- operator must first delete or backfill any rows where session_id IS NULL
-- OR contact_id IS NULL, then run:
--
--   ALTER TABLE conversations
--     MODIFY session_id VARBINARY(16) NOT NULL,
--     MODIFY contact_id VARBINARY(16) NOT NULL;

ALTER TABLE conversations
  DROP KEY uk_conversations_org_group,
  DROP KEY ix_conversations_org_type,
  DROP COLUMN group_id,
  DROP COLUMN type;
