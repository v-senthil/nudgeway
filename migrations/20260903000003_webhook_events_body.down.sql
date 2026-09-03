-- 20260903000003_webhook_events_body.down.sql
-- Reverse: drop raw_body and restore the NOT NULL constraint on raw_ref.

ALTER TABLE webhook_events
  DROP COLUMN raw_body;

ALTER TABLE webhook_events
  MODIFY raw_ref VARCHAR(255) NOT NULL DEFAULT '';
