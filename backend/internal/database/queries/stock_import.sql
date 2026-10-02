-- TEC-158: safe bulk stock import (staging, dry run, batch, undo). The
-- batch belongs to one ioengine import job; rows are staged by the preview
-- and applied/undone through ledger.Post in one transaction.

-- name: LockStockImportBatchByJob :one
SELECT * FROM stock_import_batches
WHERE import_job_id = sqlc.arg(import_job_id)
FOR UPDATE;

-- name: GetStockImportBatchByJob :one
SELECT * FROM stock_import_batches WHERE import_job_id = sqlc.arg(import_job_id);

-- name: CreateStockImportBatchForJob :one
INSERT INTO stock_import_batches (
    uuid, organization_id, brand_id, created_by_user_id, source_filename, import_job_id
)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(created_by_user_id),
    sqlc.narg(source_filename), sqlc.arg(import_job_id)
)
RETURNING *;

-- name: SetStockImportBatchState :one
UPDATE stock_import_batches
SET status = sqlc.arg(status)::text,
    rows_total = sqlc.arg(rows_total),
    rows_new = sqlc.arg(rows_new),
    rows_duplicate = sqlc.arg(rows_duplicate),
    rows_invalid = sqlc.arg(rows_invalid),
    rows_conflict = sqlc.arg(rows_conflict),
    error = sqlc.narg(error),
    applied_at = CASE WHEN sqlc.arg(status)::text = 'applied' THEN NOW() ELSE applied_at END,
    undone_at = CASE WHEN sqlc.arg(status)::text IN ('undone', 'partially_undone') THEN NOW() ELSE undone_at END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteStockImportRows :execrows
DELETE FROM stock_import_rows WHERE batch_id = sqlc.arg(batch_id);

-- name: MarkStockImportRowUndone :one
UPDATE stock_import_rows
SET row_status = 'undone',
    undo_movement_id = sqlc.arg(undo_movement_id),
    errors = '[]'::jsonb
WHERE id = sqlc.arg(id) AND row_status = 'applied'
RETURNING *;

-- name: SetStockImportRowErrors :one
UPDATE stock_import_rows
SET errors = sqlc.arg(errors)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetWarehouseLocationByCode :one
-- TEC-201: typed locations (000059) repeat sibling codes, so they are
-- addressed by full_code; legacy locations keep their organization-unique
-- code.
SELECT * FROM warehouse_locations
WHERE organization_id = sqlc.arg(organization_id)
  AND (full_code = sqlc.arg(code)::text OR (room_id IS NULL AND code = sqlc.arg(code)::text))
ORDER BY (room_id IS NOT NULL) DESC
LIMIT 1;
