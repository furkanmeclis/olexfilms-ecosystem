-- Reverts TEC-184. stock_movements is append-only: once split movements
-- exist the type CHECK cannot be narrowed again and the down fails.
DROP TABLE IF EXISTS stock_splits;

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_type CHECK (type IN (
    'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore',
    'order_out', 'order_cancel_restore', 'received', 'consumption', 'partial_consumption',
    'return', 'reclassification', 'count_adjustment', 'void', 'external_outbound'
));
