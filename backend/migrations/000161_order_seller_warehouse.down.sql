DROP INDEX IF EXISTS idx_orders_seller_warehouse;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS fk_orders_seller_warehouse;
ALTER TABLE orders DROP COLUMN IF EXISTS seller_warehouse_id;
