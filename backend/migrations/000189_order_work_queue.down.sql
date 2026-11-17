DROP INDEX IF EXISTS idx_orders_picker;
DROP INDEX IF EXISTS idx_orders_seller_queue;

ALTER TABLE orders
    DROP CONSTRAINT IF EXISTS chk_orders_weight_kg,
    DROP CONSTRAINT IF EXISTS chk_orders_desi,
    DROP CONSTRAINT IF EXISTS chk_orders_package_count,
    DROP CONSTRAINT IF EXISTS chk_orders_priority,
    DROP COLUMN IF EXISTS weight_kg,
    DROP COLUMN IF EXISTS desi,
    DROP COLUMN IF EXISTS package_count,
    DROP COLUMN IF EXISTS claimed_at,
    DROP COLUMN IF EXISTS picker_user_id,
    DROP COLUMN IF EXISTS due_date,
    DROP COLUMN IF EXISTS priority;
