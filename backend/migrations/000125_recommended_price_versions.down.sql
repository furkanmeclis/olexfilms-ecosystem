-- Reverts TEC-505. Data loss: recommended price history and price discipline
-- snapshots. product_prices.recommended_sale_price keeps the last synced
-- country-wide value; country-specific prices are lost.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'pricing.discipline.read');
DELETE FROM permissions WHERE slug = 'pricing.discipline.read';

DROP TABLE IF EXISTS price_discipline_snapshots;
DROP FUNCTION IF EXISTS price_discipline_snapshots_check_org();

DROP TRIGGER IF EXISTS trg_product_prices_sync_recommended ON product_prices;
DROP FUNCTION IF EXISTS product_prices_sync_recommended();

DROP TABLE IF EXISTS recommended_prices_current;
DROP FUNCTION IF EXISTS recommended_prices_make_current(BIGINT);
DROP FUNCTION IF EXISTS recommended_prices_current_sync_product_prices();
DROP FUNCTION IF EXISTS recommended_prices_current_check_version();

DROP TABLE IF EXISTS recommended_price_versions;
DROP FUNCTION IF EXISTS recommended_price_versions_append_only();
DROP FUNCTION IF EXISTS recommended_price_versions_check_org();
