-- TEC-1243 (kutu/metraj 6/7, karar K14): two thresholds per warehouse and
-- product. min_quantity keeps its column and now means closed boxes
-- (serial units without meters, plus fixed barcode quantities); min_meters
-- is the meter threshold over open rolls and cut pieces (remnants do not
-- count). The two alerts run apart: meter_alert_state / meter_alerted_at
-- carry the last evaluated meter state, notified on its own ok -> low edge.

ALTER TABLE stock_reorder_rules
    ADD COLUMN min_meters        NUMERIC(10,2) NULL,
    ADD COLUMN meter_alert_state VARCHAR(16)   NOT NULL DEFAULT 'ok',
    ADD COLUMN meter_alerted_at  TIMESTAMPTZ   NULL,
    ADD CONSTRAINT chk_stock_reorder_rules_min_meters CHECK (min_meters IS NULL OR min_meters > 0),
    ADD CONSTRAINT chk_stock_reorder_rules_meter_alert CHECK (meter_alert_state IN ('ok', 'low'));

ALTER TABLE stock_reorder_rules DROP CONSTRAINT chk_stock_reorder_rules_some;
ALTER TABLE stock_reorder_rules ADD CONSTRAINT chk_stock_reorder_rules_some CHECK (
    min_quantity > 0 OR reorder_point IS NOT NULL OR max_quantity IS NOT NULL
    OR warning_days IS NOT NULL OR min_meters IS NOT NULL
);
