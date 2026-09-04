-- Backfill: grant `analytics.read` to every existing admin role.
-- The permission constant was introduced with Phase 2 Analytics v1,
-- after the initial seed, so admin roles created by
-- 20260903000001 do not carry it yet. Idempotent via INSERT IGNORE.
INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'analytics.read' FROM roles WHERE name = 'admin';
