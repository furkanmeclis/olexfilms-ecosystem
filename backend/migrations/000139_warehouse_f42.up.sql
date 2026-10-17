ALTER TABLE stock_count_lines DROP CONSTRAINT chk_stock_count_lines_resolution;
ALTER TABLE stock_count_lines ADD CONSTRAINT chk_stock_count_lines_resolution
    CHECK (resolution IS NULL OR resolution IN (
        'ignore', 'relocate', 'void_missing', 'increase_unlocated', 'add_to_stock'
    ));

ALTER TABLE warehouse_transfer_lines ADD COLUMN quantity INTEGER NOT NULL DEFAULT 1
    CONSTRAINT chk_warehouse_transfer_lines_quantity CHECK (quantity > 0);
