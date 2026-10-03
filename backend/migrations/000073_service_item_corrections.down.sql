-- Reverts TEC-230: drops the consumption correction record. The ledger
-- movements it pointed to stay (stock_movements is append-only).
DROP TABLE IF EXISTS service_item_corrections;
DROP FUNCTION IF EXISTS service_item_corrections_append_only();
