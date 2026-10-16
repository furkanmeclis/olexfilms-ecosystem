-- Drops the delivery method catalog; orders lose the chosen method
-- (delivery_mode and tracking_no stay).
DROP INDEX IF EXISTS idx_orders_delivery_method;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS fk_orders_delivery_method;
ALTER TABLE orders DROP COLUMN IF EXISTS delivery_method_id;
DROP TABLE IF EXISTS delivery_methods;
DROP FUNCTION IF EXISTS delivery_methods_check_brand();
