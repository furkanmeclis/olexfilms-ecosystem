DROP INDEX IF EXISTS idx_stock_movements_from_owner;
DROP INDEX IF EXISTS idx_stock_movements_to_owner;

ALTER TABLE stock_count_lines
    DROP CONSTRAINT IF EXISTS chk_stock_count_lines_recount_rounds,
    DROP CONSTRAINT IF EXISTS chk_stock_count_lines_recount,
    DROP COLUMN IF EXISTS first_result,
    DROP COLUMN IF EXISTS first_counted_location_id,
    DROP COLUMN IF EXISTS first_counted_meters,
    DROP COLUMN IF EXISTS first_counted_quantity,
    DROP COLUMN IF EXISTS recount_rounds,
    DROP COLUMN IF EXISTS recounted_by_user_id,
    DROP COLUMN IF EXISTS recounted_at,
    DROP COLUMN IF EXISTS recount_requested_by_user_id,
    DROP COLUMN IF EXISTS recount_requested_at,
    DROP COLUMN IF EXISTS recount_status;

DROP TABLE IF EXISTS stock_reorder_rules;
