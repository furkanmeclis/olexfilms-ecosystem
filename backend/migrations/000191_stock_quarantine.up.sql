-- TEC-1226: stock adjustment documents and return quarantine (ledger part).
--
-- Status quarantine: a returned serial unit is held by the receiving
-- organization but waits for inspection; it is not counted as stock
-- (projections count available and placed only), so it cannot be picked,
-- reserved, transferred or used in a service until it is released.
--
-- Movement types:
--   quarantine          available/placed -> quarantine (owner kept)
--   quarantine_release  quarantine -> available (owner kept; a placement may follow)
--   found               void (holder trash) -> available (adjustment document "found")
-- void and external_outbound also accept a quarantined unit (scrap, back
-- to the supplier).

ALTER TABLE units DROP CONSTRAINT chk_units_status;
ALTER TABLE units ADD CONSTRAINT chk_units_status CHECK (status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void', 'quarantine'));

ALTER TABLE unit_current_state DROP CONSTRAINT chk_unit_current_state_status;
ALTER TABLE unit_current_state ADD CONSTRAINT chk_unit_current_state_status CHECK (status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void', 'quarantine'));

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_from_status;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_from_status CHECK (from_status IS NULL OR from_status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void', 'quarantine'));

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_to_status;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_to_status CHECK (to_status IS NULL OR to_status IN (
    'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void', 'quarantine'));

ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_type CHECK (type IN (
    'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore', 'order_out', 'received',
    'order_cancel_restore', 'consumption', 'partial_consumption', 'return', 'sale', 'count_adjustment', 'void',
    'external_outbound', 'reclassification', 'split', 'measure_conversion',
    'quarantine', 'quarantine_release', 'found'));
