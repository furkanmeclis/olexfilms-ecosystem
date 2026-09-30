INSERT INTO roles (name, slug, description, is_system) VALUES
    ('CMS User', 'cms_user', 'Default CMS access for content users', false)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('auth.session', 'notifications.read')
WHERE r.slug = 'cms_user'
ON CONFLICT DO NOTHING;
