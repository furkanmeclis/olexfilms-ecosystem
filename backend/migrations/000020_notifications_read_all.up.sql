INSERT INTO permissions (name, slug)
VALUES ('Read all platform notifications', 'platform.notifications.read_all')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'platform.notifications.read_all'
WHERE r.slug = 'super_admin'
ON CONFLICT DO NOTHING;
