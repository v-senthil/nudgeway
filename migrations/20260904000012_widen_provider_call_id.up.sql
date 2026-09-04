-- Widen calls.provider_call_id — Meta's wacid values are ~82 chars for
-- some webhook events (base64-ish opaque handle), overflowing VARCHAR(64)
-- and rejecting every inbound call webhook with error 1406.
ALTER TABLE calls
  MODIFY provider_call_id VARCHAR(255) NOT NULL;
