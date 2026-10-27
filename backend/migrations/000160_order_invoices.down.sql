DROP TABLE IF EXISTS order_invoice_items;
DROP TABLE IF EXISTS order_invoices;
DROP INDEX IF EXISTS idx_orders_invoice_status;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_invoice_status;
ALTER TABLE orders DROP COLUMN IF EXISTS invoice_status;
