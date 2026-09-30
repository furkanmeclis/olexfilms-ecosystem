DELETE FROM user_roles
WHERE role_id IN (SELECT id FROM roles WHERE slug = 'organization_owner');

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE slug = 'organization_owner');

DELETE FROM roles WHERE slug = 'organization_owner';
