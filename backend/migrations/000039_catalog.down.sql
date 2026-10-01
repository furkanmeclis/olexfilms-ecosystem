DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('catalog.read', 'catalog.write'));
DELETE FROM permissions WHERE slug IN ('catalog.read', 'catalog.write');

DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS product_categories;
DROP FUNCTION IF EXISTS catalog_check_center_org();

ALTER TABLE brands DROP CONSTRAINT IF EXISTS chk_brands_currency;
ALTER TABLE brands DROP COLUMN IF EXISTS currency;
