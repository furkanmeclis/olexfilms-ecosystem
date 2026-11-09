DROP INDEX IF EXISTS idx_order_items_plotter_cut;
ALTER TABLE order_items DROP COLUMN IF EXISTS plotter_cut;
