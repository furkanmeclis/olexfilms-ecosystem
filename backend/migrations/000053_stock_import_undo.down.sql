-- Reverts TEC-158. Undone rows and partially undone batches fall back to
-- applied (their reverse movements stay: stock_movements is append-only).
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug = 'center_warehouse'
  AND p.slug = 'tenant.imports.read';

ALTER TABLE stock_import_rows DROP CONSTRAINT IF EXISTS chk_stock_import_rows_undo;
UPDATE stock_import_rows SET row_status = 'applied' WHERE row_status = 'undone';
ALTER TABLE stock_import_rows DROP CONSTRAINT chk_stock_import_rows_status;
ALTER TABLE stock_import_rows ADD CONSTRAINT chk_stock_import_rows_status CHECK (row_status IN (
    'new', 'duplicate', 'invalid', 'conflict', 'applied', 'skipped'
));
ALTER TABLE stock_import_rows DROP COLUMN IF EXISTS undo_movement_id;

UPDATE stock_import_batches SET status = 'applied' WHERE status = 'partially_undone';
ALTER TABLE stock_import_batches DROP CONSTRAINT chk_stock_import_batches_status;
ALTER TABLE stock_import_batches ADD CONSTRAINT chk_stock_import_batches_status CHECK (status IN (
    'draft', 'validated', 'applying', 'applied', 'failed', 'undone'
));
DROP INDEX IF EXISTS uq_stock_import_batches_import_job;
ALTER TABLE stock_import_batches DROP COLUMN IF EXISTS import_job_id;
