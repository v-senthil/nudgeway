-- Reverse of the backfill: revoke `analytics.read` from every admin role.
DELETE rp FROM role_permissions rp
JOIN roles r ON r.id = rp.role_id
WHERE r.name = 'admin' AND rp.permission_key = 'analytics.read';
