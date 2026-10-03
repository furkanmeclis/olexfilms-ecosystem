-- Reverts TEC-229.
DROP INDEX IF EXISTS idx_accounting_disputes_source_reversal;
DROP INDEX IF EXISTS idx_stock_transfer_request_items_order_item;
ALTER TABLE stock_transfer_request_items
    DROP COLUMN IF EXISTS accounting_excluded,
    DROP COLUMN IF EXISTS order_item_id;
