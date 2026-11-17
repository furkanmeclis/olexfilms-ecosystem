DROP FUNCTION IF EXISTS unit_credits_backfill();
DROP FUNCTION IF EXISTS unit_credit_default(BIGINT);
DROP TABLE IF EXISTS unit_credit_balances;
DROP TABLE IF EXISTS unit_credit_movements;
DROP FUNCTION IF EXISTS unit_credit_movements_append_only();

ALTER TABLE service_items DROP COLUMN IF EXISTS finished_product;

ALTER TABLE order_items
    DROP CONSTRAINT IF EXISTS chk_order_items_plotter_credits,
    DROP COLUMN IF EXISTS plotter_credits;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS chk_products_usage_credits,
    DROP COLUMN IF EXISTS usage_credits;
