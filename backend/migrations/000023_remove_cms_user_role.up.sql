UPDATE auth_settings
SET default_role_id = NULL
WHERE default_role_id IN (SELECT id FROM roles WHERE slug = 'cms_user');

DELETE FROM user_roles
WHERE role_id IN (SELECT id FROM roles WHERE slug = 'cms_user');

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE slug = 'cms_user');

DELETE FROM roles WHERE slug = 'cms_user';
