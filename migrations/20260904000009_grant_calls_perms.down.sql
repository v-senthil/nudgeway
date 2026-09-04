-- Reverse of the calls-permission backfill: revoke both keys from admin roles.
DELETE rp FROM role_permissions rp
JOIN roles r ON r.id = rp.role_id
WHERE r.name = 'admin' AND rp.permission_key IN ('calls.read', 'calls.manage');
