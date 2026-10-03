-- TEC-204: stock entry documents (draft -> lines -> place -> confirm) and
-- the entry written by an applied stock import (TEC-158).

-- name: CreateStockEntry :one
INSERT INTO stock_entries (
    organization_id, brand_id, warehouse_id, mode, note, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(warehouse_id), sqlc.arg(mode),
    sqlc.narg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: CreateImportStockEntry :one
-- An applied import batch is a confirmed entry at once.
INSERT INTO stock_entries (
    organization_id, brand_id, mode, status, import_batch_id,
    created_by_user_id, confirmed_by_user_id, confirmed_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), 'import', 'confirmed', sqlc.arg(import_batch_id),
    sqlc.narg(user_id), sqlc.narg(user_id), NOW()
)
ON CONFLICT (import_batch_id) DO NOTHING
RETURNING *;

-- name: GetStockEntryByUUID :one
SELECT * FROM stock_entries
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: LockStockEntryByUUID :one
SELECT * FROM stock_entries
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: GetStockEntryByImportBatch :one
SELECT * FROM stock_entries WHERE import_batch_id = sqlc.arg(import_batch_id) FOR UPDATE;

-- name: ListStockEntries :many
SELECT * FROM stock_entries
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountStockEntries :one
SELECT count(*) FROM stock_entries
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: ConfirmStockEntry :one
UPDATE stock_entries
SET status = 'confirmed', confirmed_at = NOW(), confirmed_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: CancelStockEntry :one
UPDATE stock_entries
SET status = 'cancelled', cancelled_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: MarkStockEntryUndone :one
UPDATE stock_entries
SET status = 'undone'
WHERE id = sqlc.arg(id) AND status = 'confirmed'
RETURNING *;

-- name: InsertStockEntryLine :one
INSERT INTO stock_entry_lines (
    entry_id, unit_id, quantity, location_id, entry_movement_id
) VALUES (
    sqlc.arg(entry_id), sqlc.arg(unit_id), sqlc.arg(quantity),
    sqlc.narg(location_id), sqlc.narg(entry_movement_id)
)
RETURNING *;

-- name: ListStockEntryLines :many
SELECT * FROM stock_entry_lines WHERE entry_id = sqlc.arg(entry_id) ORDER BY id;

-- name: CountStockEntryLines :one
SELECT count(*) FROM stock_entry_lines WHERE entry_id = sqlc.arg(entry_id);

-- name: DeleteStockEntryLine :execrows
DELETE FROM stock_entry_lines
WHERE uuid = sqlc.arg(uuid) AND entry_id = sqlc.arg(entry_id);

-- name: SetStockEntryLineLocation :execrows
UPDATE stock_entry_lines SET location_id = sqlc.arg(location_id)
WHERE id = ANY(sqlc.arg(ids)::bigint[]) AND entry_id = sqlc.arg(entry_id);

-- name: SetStockEntryLineMovements :exec
UPDATE stock_entry_lines
SET entry_movement_id = sqlc.arg(entry_movement_id),
    placement_movement_id = sqlc.narg(placement_movement_id)
WHERE id = sqlc.arg(id);

-- name: SetStockEntryLineUndone :exec
UPDATE stock_entry_lines SET undo_movement_id = sqlc.arg(undo_movement_id)
WHERE entry_id = sqlc.arg(entry_id) AND unit_id = sqlc.arg(unit_id);

-- name: FindOpenStockEntryForUnit :one
-- Another draft entry (or, for serial units, a confirmed one whose line
-- was not undone) already holding the unit.
SELECT e.uuid, e.status FROM stock_entry_lines l
JOIN stock_entries e ON e.id = l.entry_id
WHERE l.unit_id = sqlc.arg(unit_id) AND l.undo_movement_id IS NULL
  AND (e.status = 'draft' OR (sqlc.arg(include_confirmed)::boolean AND e.status = 'confirmed'))
LIMIT 1;

-- name: GetUnitBatchUUID :one
SELECT b.uuid FROM barcode_batches b WHERE b.id = sqlc.arg(id);

-- name: GetStockImportBatchUUID :one
SELECT uuid FROM stock_import_batches WHERE id = sqlc.arg(id);
