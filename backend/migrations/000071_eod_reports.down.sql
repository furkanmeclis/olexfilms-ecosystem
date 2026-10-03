-- Reverts TEC-207. Data loss: every stored end-of-day report (the
-- summaries are derived from stock_movements and can be regenerated).
DROP TABLE IF EXISTS eod_reports;
DROP INDEX IF EXISTS idx_stock_movements_created;
