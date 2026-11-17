DROP INDEX IF EXISTS idx_orders_parent_order;

ALTER TABLE orders
    DROP CONSTRAINT IF EXISTS chk_orders_parent_not_self,
    DROP CONSTRAINT IF EXISTS fk_orders_parent_order,
    DROP COLUMN IF EXISTS parent_order_id;
