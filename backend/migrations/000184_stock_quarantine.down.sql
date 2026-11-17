-- The ledger is append-only: rolling back is refused while a quarantine
-- movement or a quarantined unit exists (the old constraints would not hold).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM stock_movements WHERE type IN ('quarantine', 'quarantine_release', 'found'))
       OR EXISTS (SELECT 1 FROM units WHERE status = 'quarantine') THEN
        RAISE EXCEPTION '000184 down: quarantine/found movements exist';
    END IF;
END $$;

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_type CHECK (type IN (
    'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore', 'order_out', 'received',
    'order_cancel_restore', 'consumption', 'partial_consumption', 'return', 'sale', 'count_adjustment', 'void',
    'external_outbound', 'reclassification', 'split', 'measure_conversion'));

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_to_status;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_to_status CHECK (to_status IS NULL OR to_status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'));

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_from_status;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_from_status CHECK (from_status IS NULL OR from_status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'));

ALTER TABLE unit_current_state DROP CONSTRAINT chk_unit_current_state_status;
ALTER TABLE unit_current_state ADD CONSTRAINT chk_unit_current_state_status CHECK (status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'));

ALTER TABLE units DROP CONSTRAINT chk_units_status;
ALTER TABLE units ADD CONSTRAINT chk_units_status CHECK (status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'));
