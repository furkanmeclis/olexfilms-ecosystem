DELETE FROM stock_reorder_rules
WHERE min_quantity = 0 AND reorder_point IS NULL AND max_quantity IS NULL AND warning_days IS NULL;

ALTER TABLE stock_reorder_rules DROP CONSTRAINT chk_stock_reorder_rules_some;
ALTER TABLE stock_reorder_rules ADD CONSTRAINT chk_stock_reorder_rules_some CHECK (
    min_quantity > 0 OR reorder_point IS NOT NULL OR max_quantity IS NOT NULL OR warning_days IS NOT NULL
);

ALTER TABLE stock_reorder_rules
    DROP CONSTRAINT chk_stock_reorder_rules_meter_alert,
    DROP CONSTRAINT chk_stock_reorder_rules_min_meters,
    DROP COLUMN meter_alerted_at,
    DROP COLUMN meter_alert_state,
    DROP COLUMN min_meters;
