ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements
    ADD CONSTRAINT chk_stock_movements_type CHECK (
        type IN (
            'entry', 'placement', 'transfer_out', 'transfer_in',
            'transfer_cancel_restore', 'order_out', 'received',
            'order_cancel_restore', 'consumption', 'partial_consumption',
            'return', 'count_adjustment', 'void', 'external_outbound',
            'reclassification', 'split'
        )
    );
