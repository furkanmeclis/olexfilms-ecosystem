DROP TABLE IF EXISTS warehouse_transfer_discrepancies;
DROP TABLE IF EXISTS order_receipt_discrepancies;
DROP INDEX IF EXISTS idx_orders_backorder_of;
ALTER TABLE orders
    DROP CONSTRAINT IF EXISTS chk_orders_backorder_not_self,
    DROP CONSTRAINT IF EXISTS fk_orders_backorder_of,
    DROP COLUMN IF EXISTS backorder_of_order_id;
