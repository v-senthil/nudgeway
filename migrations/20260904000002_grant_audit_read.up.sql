-- Backfill: grant `audit.read` to every existing admin role.
-- The permission constant was introduced after the initial seed, so
-- admin roles created by 20260903000001 do not have it yet.
-- Idempotent via INSERT IGNORE.
INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'audit.read' FROM roles WHERE name = 'admin';
