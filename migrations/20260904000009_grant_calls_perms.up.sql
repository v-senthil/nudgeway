-- 20260904000005_grant_calls_perms.up.sql
-- Backfill: grant `calls.read` and `calls.manage` to every existing admin
-- role. Idempotent via INSERT IGNORE. Landed alongside the WhatsApp
-- Calling vertical slice so operators of pre-existing tenants can access
-- the new UI without a re-seed.
INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'calls.read' FROM roles WHERE name = 'admin';

INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'calls.manage' FROM roles WHERE name = 'admin';
