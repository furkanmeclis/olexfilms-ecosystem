-- TEC-1225: receiving exceptions (discrepancy record, attachments, line
-- production date / lot, undo of a confirmed manual entry).

-- name: SetStockEntryLineDetails :one
UPDATE stock_entry_lines
SET production_date = sqlc.narg(production_date)::date,
    box_no = sqlc.narg(box_no)::varchar
WHERE uuid = sqlc.arg(uuid) AND entry_id = sqlc.arg(entry_id)
RETURNING *;

-- name: SetStockEntryLinesProductionDate :exec
UPDATE stock_entry_lines SET production_date = sqlc.narg(production_date)::date
WHERE entry_id = sqlc.arg(entry_id) AND unit_id = ANY(sqlc.arg(unit_ids)::bigint[]);

-- name: DeleteStockEntryLineByID :execrows
DELETE FROM stock_entry_lines WHERE id = sqlc.arg(id) AND entry_id = sqlc.arg(entry_id);

-- name: InsertStockEntryAttachment :one
INSERT INTO stock_entry_attachments (
    organization_id, brand_id, entry_id, kind, storage_key, filename, mime_type, size_bytes, uploaded_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(entry_id), sqlc.arg(kind), sqlc.arg(storage_key),
    sqlc.arg(filename), sqlc.arg(mime_type), sqlc.arg(size_bytes), sqlc.narg(uploaded_by_user_id)
)
RETURNING *;

-- name: ListStockEntryAttachments :many
SELECT a.*, u.uuid AS uploader_uuid, u.name AS uploader_name, u.surname AS uploader_surname
FROM stock_entry_attachments a
LEFT JOIN users u ON u.id = a.uploaded_by_user_id
WHERE a.entry_id = sqlc.arg(entry_id) AND a.deleted_at IS NULL
ORDER BY a.id;

-- name: GetStockEntryAttachment :one
SELECT * FROM stock_entry_attachments
WHERE uuid = sqlc.arg(uuid) AND entry_id = sqlc.arg(entry_id) AND deleted_at IS NULL;

-- name: SoftDeleteStockEntryAttachment :execrows
UPDATE stock_entry_attachments SET deleted_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: InsertStockEntryDiscrepancy :one
INSERT INTO stock_entry_discrepancies (
    organization_id, brand_id, entry_id, kind, decision, barcode, unit_id, product_id,
    quantity, reason, attachment_ids, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(entry_id), sqlc.arg(kind), sqlc.arg(decision),
    sqlc.arg(barcode), sqlc.narg(unit_id), sqlc.narg(product_id), sqlc.arg(quantity), sqlc.narg(reason),
    sqlc.arg(attachment_ids)::bigint[], sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: ListStockEntryDiscrepancies :many
SELECT d.*, p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname
FROM stock_entry_discrepancies d
LEFT JOIN products p ON p.id = d.product_id
LEFT JOIN users u ON u.id = d.created_by_user_id
WHERE d.entry_id = sqlc.arg(entry_id)
ORDER BY d.id;

-- name: CountStockEntryAttachmentRefs :one
-- Discrepancies that name the attachment as damage evidence.
SELECT count(*) FROM stock_entry_discrepancies
WHERE entry_id = sqlc.arg(entry_id) AND sqlc.arg(attachment_id)::bigint = ANY(attachment_ids);

-- name: ListUnitMovementsAfter :many
-- Movements of a unit after the given one (the undo check of an entry).
SELECT m.id, m.uuid, m.type, m.reference_type, m.created_at
FROM stock_movements m
WHERE m.unit_id = sqlc.arg(unit_id) AND m.id > sqlc.arg(after_id)
ORDER BY m.id
LIMIT 5;

-- name: MarkManualStockEntryUndone :one
UPDATE stock_entries
SET status = 'undone', undone_at = NOW(), undone_by_user_id = sqlc.narg(user_id), undo_reason = sqlc.narg(reason)
WHERE id = sqlc.arg(id) AND status = 'confirmed'
RETURNING *;

-- name: SetStockEntryLineUndoMovement :exec
UPDATE stock_entry_lines SET undo_movement_id = sqlc.arg(undo_movement_id)
WHERE id = sqlc.arg(id);

-- name: GetStockEntryUserRef :one
SELECT uuid, name, surname FROM users WHERE id = sqlc.arg(id);

-- name: GetStockEntrySupplierDetails :one
SELECT uuid, name, tax_no, phone_e164, email FROM suppliers WHERE id = sqlc.arg(id);
