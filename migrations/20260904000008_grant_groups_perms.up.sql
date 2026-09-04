-- Backfill: grant `groups.read` + `groups.manage` to every existing admin role.
-- The permission constants were introduced with the Groups feature; admin
-- roles created by earlier migrations do not have them yet.
-- Idempotent via INSERT IGNORE.
INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'groups.read' FROM roles WHERE name = 'admin';

INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'groups.manage' FROM roles WHERE name = 'admin';
