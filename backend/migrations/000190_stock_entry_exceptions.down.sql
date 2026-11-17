-- Drops the TEC-1225 receiving records (discrepancies, attachment rows; the
-- stored objects stay in object storage), the production dates and the undo
-- details of entries.
DROP TRIGGER IF EXISTS trg_stock_entry_discrepancies_append_only ON stock_entry_discrepancies;
DROP FUNCTION IF EXISTS stock_entry_discrepancies_append_only();
DROP TABLE IF EXISTS stock_entry_discrepancies;
DROP TABLE IF EXISTS stock_entry_attachments;
ALTER TABLE stock_entries
    DROP COLUMN IF EXISTS undo_reason,
    DROP COLUMN IF EXISTS undone_by_user_id,
    DROP COLUMN IF EXISTS undone_at;
ALTER TABLE stock_entry_lines
    DROP COLUMN IF EXISTS production_date;
