UPDATE role_permissions rp
SET scope = 'managed'
FROM roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug IN ('distributor_owner', 'distributor_warehouse_staff')
  AND p.slug = 'stock.read'
  AND rp.scope = 'subtree';
