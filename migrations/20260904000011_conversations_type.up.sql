-- 20260904000011_conversations_type.up.sql
-- Introduce conversation "type" (one_to_one | group) so a Group can own a
-- Conversation row that appears in the inbox alongside 1-to-1 threads.
--
-- Design notes:
--   * `type` defaults to 'one_to_one' so existing rows keep behaving
--     exactly as before — no backfill required.
--   * `group_id` is nullable and only populated when type='group'.
--   * `session_id` / `contact_id` are relaxed to NULL so group-typed rows
--     can persist without a session / contact identity. 1-to-1 rows keep
--     both populated at insert time.
--   * (org_id, group_id) is unique so a repeated Create call cannot double
--     the row — the application service also short-circuits via lookup.
--   * No FK to groups(id) here — keeping the migration self-contained.

ALTER TABLE conversations
  ADD COLUMN type ENUM('one_to_one','group') NOT NULL DEFAULT 'one_to_one' AFTER contact_id,
  ADD COLUMN group_id VARBINARY(16) NULL AFTER type,
  MODIFY session_id VARBINARY(16) NULL,
  MODIFY contact_id VARBINARY(16) NULL,
  ADD UNIQUE KEY uk_conversations_org_group (org_id, group_id),
  ADD KEY ix_conversations_org_type (org_id, type);
