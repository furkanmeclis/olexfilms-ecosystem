DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('platform.organizations.read', 'platform.organizations.write')
);

DELETE FROM permissions
WHERE slug IN ('platform.organizations.read', 'platform.organizations.write');

DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
