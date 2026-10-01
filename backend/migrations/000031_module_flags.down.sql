DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.slug IN ('modules.read', 'modules.manage', 'platform.modules.read', 'platform.modules.write');

DELETE FROM permissions
WHERE slug IN ('modules.read', 'modules.manage', 'platform.modules.read', 'platform.modules.write');

DROP TABLE IF EXISTS module_flags;
DROP TABLE IF EXISTS modules;
