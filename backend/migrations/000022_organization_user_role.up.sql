INSERT INTO roles (name, slug, description, is_system) VALUES
    ('Organization', 'organization_user', 'Business tenant access for organization owners and staff', true);

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('auth.session', 'notifications.read')
WHERE r.slug = 'organization_user';

-- Backfill existing organization members.
INSERT INTO user_roles (user_id, role_id)
SELECT DISTINCT om.user_id, r.id
FROM organization_members om
JOIN roles r ON r.slug = 'organization_user'
ON CONFLICT DO NOTHING;
