-- 20260903000003_webhook_events_body.up.sql
-- Persist raw webhook body inline for replay + debugging, and relax the
-- raw_ref column (previously NOT NULL DEFAULT '') so callers can leave it
-- empty when the body itself is stored in raw_body.

ALTER TABLE webhook_events
  ADD COLUMN raw_body MEDIUMBLOB NULL AFTER raw_ref;

ALTER TABLE webhook_events
  MODIFY raw_ref VARCHAR(255) NULL;
