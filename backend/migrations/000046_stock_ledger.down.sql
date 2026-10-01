-- Reverts TEC-153. Data loss: every unit, stock movement, projection,
-- reclassification request and import batch.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug LIKE 'stock.%');
DELETE FROM permissions WHERE slug IN (
    'stock.read', 'stock.write', 'stock.adjust', 'stock.reclassify', 'stock.import'
);

DROP TABLE IF EXISTS stock_import_rows;
DROP TABLE IF EXISTS stock_import_batches;
DROP TABLE IF EXISTS stock_reclassifications;
DROP TABLE IF EXISTS organization_product_stocks;
DROP TABLE IF EXISTS bin_product_stocks;
DROP TABLE IF EXISTS fixed_barcode_holdings;
DROP TABLE IF EXISTS unit_current_state;
DROP TABLE IF EXISTS stock_movements;
DROP FUNCTION IF EXISTS stock_movements_append_only();
DROP TABLE IF EXISTS units;
DROP FUNCTION IF EXISTS units_check_integrity();
DROP TABLE IF EXISTS warehouse_locations;
