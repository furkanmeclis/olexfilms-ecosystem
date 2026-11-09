DROP INDEX IF EXISTS idx_stock_transfer_requests_order;

ALTER TABLE stock_transfer_requests
    DROP CONSTRAINT IF EXISTS chk_stock_transfer_requests_opened_by_parent,
    DROP CONSTRAINT IF EXISTS chk_stock_transfer_requests_order_return,
    DROP CONSTRAINT IF EXISTS fk_stock_transfer_requests_order,
    DROP COLUMN IF EXISTS opened_by_parent,
    DROP COLUMN IF EXISTS order_id;
