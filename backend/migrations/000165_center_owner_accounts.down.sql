DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id AND rp.permission_id = p.id
  AND r.slug = 'center_owner' AND p.slug IN ('members.write', 'organizations.write');
