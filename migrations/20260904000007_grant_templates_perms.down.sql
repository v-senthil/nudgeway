-- 20260904000004_grant_templates_perms.down.sql
-- Reverse of the backfill: revoke the templates permissions from every
-- admin role.
DELETE rp FROM role_permissions rp
JOIN roles r ON r.id = rp.role_id
WHERE r.name = 'admin' AND rp.permission_key IN ('templates.read', 'templates.manage');
