-- 20260904000004_grant_templates_perms.up.sql
-- Backfill: grant `templates.read` + `templates.manage` to every existing
-- admin role. The permission constants ship with the templates feature in
-- Phase 2 — admin roles created by the initial seed do not have them yet.
-- Idempotent via INSERT IGNORE.
INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'templates.read' FROM roles WHERE name = 'admin';

INSERT IGNORE INTO role_permissions (role_id, permission_key)
SELECT id, 'templates.manage' FROM roles WHERE name = 'admin';
