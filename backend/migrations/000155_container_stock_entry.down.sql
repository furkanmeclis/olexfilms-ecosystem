DROP TABLE IF EXISTS import_mapping_templates;
DROP TABLE IF EXISTS supplier_product_codes;
DROP TABLE IF EXISTS stock_entry_scans;

ALTER TABLE stock_entry_lines
    DROP CONSTRAINT IF EXISTS chk_stock_entry_lines_scanned,
    DROP COLUMN IF EXISTS scanned_by,
    DROP COLUMN IF EXISTS scanned_at,
    DROP COLUMN IF EXISTS box_no;

DROP INDEX IF EXISTS idx_stock_entries_container;
ALTER TABLE stock_entries
    DROP CONSTRAINT IF EXISTS chk_stock_entries_waybill_no,
    DROP CONSTRAINT IF EXISTS chk_stock_entries_container_no,
    DROP CONSTRAINT IF EXISTS fk_stock_entries_supplier,
    DROP COLUMN IF EXISTS default_location_id,
    DROP COLUMN IF EXISTS arrival_date,
    DROP COLUMN IF EXISTS waybill_no,
    DROP COLUMN IF EXISTS container_no,
    DROP COLUMN IF EXISTS supplier_id;
