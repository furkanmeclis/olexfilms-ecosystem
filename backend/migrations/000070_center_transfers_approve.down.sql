-- Reverts TEC-228: center roles lose transfers.approve again.
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug IN ('center_staff', 'center_warehouse')
  AND p.slug = 'transfers.approve';
