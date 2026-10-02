-- TEC-216 (F1-12a): the distributor reads the stock of its dealers (K4,
-- design §4). Only stock.read widens to subtree, and only for the two
-- distributor roles that hold stock permissions; stock.write and
-- stock.adjust stay managed (the distributor never moves dealer stock).
-- distributor_staff, distributor_accounting and dealer roles are untouched.
-- Source of truth: internal/platform/rbac/catalog.go.
UPDATE role_permissions rp
SET scope = 'subtree'
FROM roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug IN ('distributor_owner', 'distributor_warehouse_staff')
  AND p.slug = 'stock.read'
  AND rp.scope = 'managed';
