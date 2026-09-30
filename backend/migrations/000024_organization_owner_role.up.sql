-- Organization owner role (base migration 000024 without the
-- tenant finance permission seed; business permissions arrive with their
-- own modules).
INSERT INTO roles (name, slug, description, is_system) VALUES
    ('Organization Owner', 'organization_owner', 'Organization owner', true)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'auth.session',
    'notifications.read'
)
WHERE r.slug = 'organization_owner'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT DISTINCT om.user_id, r.id
FROM organization_members om
JOIN roles r ON r.slug = 'organization_owner'
WHERE om.role = 'owner'
ON CONFLICT DO NOTHING;
