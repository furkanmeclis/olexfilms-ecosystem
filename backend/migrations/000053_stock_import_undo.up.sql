-- TEC-158 (F1-02f): safe bulk stock import. The staging tables of 000046
-- gain what the ioengine flow and the controlled undo need:
--
--   * stock_import_batches.import_job_id: the ioengine import job the batch
--     belongs to (one batch per job; the batch uuid is the job uuid).
--   * batch status partially_undone: the undo reversed some rows and refused
--     the rows whose units moved after the import.
--   * row status undone + undo_movement_id: the reverse (void) movement the
--     undo wrote through ledger.Post. A refused row stays applied and keeps
--     the reason in errors.
ALTER TABLE stock_import_batches
    ADD COLUMN import_job_id BIGINT NULL REFERENCES import_jobs (id) ON DELETE SET NULL;

CREATE UNIQUE INDEX uq_stock_import_batches_import_job
    ON stock_import_batches (import_job_id) WHERE import_job_id IS NOT NULL;

ALTER TABLE stock_import_batches DROP CONSTRAINT chk_stock_import_batches_status;
ALTER TABLE stock_import_batches ADD CONSTRAINT chk_stock_import_batches_status CHECK (status IN (
    'draft', 'validated', 'applying', 'applied', 'failed', 'undone', 'partially_undone'
));

ALTER TABLE stock_import_rows
    ADD COLUMN undo_movement_id BIGINT NULL REFERENCES stock_movements (id) ON DELETE RESTRICT;

ALTER TABLE stock_import_rows DROP CONSTRAINT chk_stock_import_rows_status;
ALTER TABLE stock_import_rows ADD CONSTRAINT chk_stock_import_rows_status CHECK (row_status IN (
    'new', 'duplicate', 'invalid', 'conflict', 'applied', 'skipped', 'undone'
));
ALTER TABLE stock_import_rows ADD CONSTRAINT chk_stock_import_rows_undo CHECK (
    (row_status = 'undone') = (undo_movement_id IS NOT NULL)
);

-- Stock import jobs are previewed, confirmed and undone through
-- /v1/tenant/imports (tenant.imports.read); center_warehouse holds
-- stock.import, so it gets the job routes too.
-- Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r
JOIN permissions p ON p.slug = 'tenant.imports.read'
WHERE r.slug = 'center_warehouse'
ON CONFLICT DO NOTHING;
