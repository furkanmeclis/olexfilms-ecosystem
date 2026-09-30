DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug = 'platform.notifications.read_all'
);

DELETE FROM permissions WHERE slug = 'platform.notifications.read_all';
