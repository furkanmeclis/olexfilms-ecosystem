-- Reverts TEC-85. Data loss: per-membership roles (organization_member_roles),
-- the new system roles and the business permissions are dropped; grant scopes
-- are discarded.

DROP TRIGGER IF EXISTS trg_role_permissions_check_scope ON role_permissions;
DROP FUNCTION IF EXISTS check_role_permission_scope();

-- Legacy global organization roles (000022 / 000024 / 000026).
INSERT INTO roles (name, slug, description, is_system) VALUES
    ('Organization', 'organization_user', 'Business tenant access for organization owners and staff', true),
    ('Organization Owner', 'organization_owner', 'Organization owner', true)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('auth.session', 'notifications.read')
WHERE r.slug IN ('organization_user', 'organization_owner')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.settings.read', 'tenant.settings.write', 'tenant.imports.read')
WHERE r.slug = 'organization_owner'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT DISTINCT om.user_id, r.id
FROM organization_members om
JOIN roles r ON r.slug = 'organization_user'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT DISTINCT om.user_id, r.id
FROM organization_members om
JOIN roles r ON r.slug = 'organization_owner'
WHERE om.role = 'owner'
ON CONFLICT DO NOTHING;

DROP TABLE IF EXISTS organization_member_roles;

DELETE FROM roles WHERE slug IN (
    'center_staff', 'center_warehouse', 'center_accounting', 'center_social',
    'distributor_owner', 'distributor_staff', 'distributor_warehouse_staff', 'distributor_accounting',
    'dealer_owner', 'dealer_staff', 'dealer_accounting',
    'customer', 'fleet'
);

DELETE FROM permissions WHERE slug IN (
    'tenant.exports.read',
    'organizations.read', 'organizations.write', 'organizations.supplier.write',
    'members.read', 'members.write',
    'services.read', 'services.write', 'customers.read', 'customers.write',
    'pricing.purchase.read', 'pricing.sale.read', 'pricing.sale.write',
    'pricing.recommended.read', 'pricing.recommended.write',
    'accounting.read', 'accounting.write', 'warehouse.read', 'warehouse.write',
    'campaigns.read', 'campaigns.write', 'leads.read', 'leads.write',
    'social.read', 'social.write', 'privacy.anonymize'
);

-- Restore permission names changed by the catalog.
UPDATE permissions SET name = 'Read tenant export settings' WHERE slug = 'tenant.settings.read';
UPDATE permissions SET name = 'Write tenant export settings' WHERE slug = 'tenant.settings.write';
UPDATE permissions SET name = 'Read tenant import jobs' WHERE slug = 'tenant.imports.read';

-- super_admin holds every permission (as before).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin'
ON CONFLICT DO NOTHING;

ALTER TABLE role_permissions
    DROP CONSTRAINT IF EXISTS chk_role_permissions_scope,
    DROP COLUMN IF EXISTS scope;

ALTER TABLE roles
    DROP CONSTRAINT IF EXISTS chk_roles_org_type,
    DROP COLUMN IF EXISTS org_type;

DROP INDEX IF EXISTS idx_permissions_module;

ALTER TABLE permissions
    DROP CONSTRAINT IF EXISTS chk_permissions_scopes,
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS super_admin_only,
    DROP COLUMN IF EXISTS is_sensitive,
    DROP COLUMN IF EXISTS scopes,
    DROP COLUMN IF EXISTS module;
