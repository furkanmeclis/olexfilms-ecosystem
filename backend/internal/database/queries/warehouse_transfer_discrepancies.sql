-- TEC-1227: the discrepancy record (tutanak) of a warehouse transfer.

-- name: CreateWarehouseTransferDiscrepancy :one
INSERT INTO warehouse_transfer_discrepancies (
    organization_id, brand_id, transfer_id, transfer_line_id, unit_id, barcode, kind, note, created_by_user_id
)
SELECT t.organization_id, t.brand_id, t.id, sqlc.narg(transfer_line_id), sqlc.narg(unit_id),
       sqlc.arg(barcode), sqlc.arg(kind), sqlc.narg(note), sqlc.narg(created_by_user_id)
FROM warehouse_transfers t
WHERE t.id = sqlc.arg(transfer_id)
RETURNING *;

-- name: ListWarehouseTransferDiscrepancies :many
SELECT d.*, u.uuid AS unit_uuid, p.name AS product_name, p.sku AS product_sku,
       cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname,
       ru.uuid AS resolved_by_uuid, ru.name AS resolved_by_name, ru.surname AS resolved_by_surname
FROM warehouse_transfer_discrepancies d
LEFT JOIN units u ON u.id = d.unit_id
LEFT JOIN products p ON p.id = u.product_id
LEFT JOIN users cu ON cu.id = d.created_by_user_id
LEFT JOIN users ru ON ru.id = d.resolved_by_user_id
WHERE d.transfer_id = sqlc.arg(transfer_id)
ORDER BY d.id;

-- name: LockWarehouseTransferDiscrepancy :one
SELECT * FROM warehouse_transfer_discrepancies
WHERE uuid = sqlc.arg(uuid) AND transfer_id = sqlc.arg(transfer_id)
FOR UPDATE;

-- name: ResolveWarehouseTransferDiscrepancy :one
UPDATE warehouse_transfer_discrepancies
SET status = 'resolved',
    resolution = sqlc.arg(resolution),
    resolution_note = sqlc.narg(resolution_note),
    resolved_by_user_id = sqlc.narg(resolved_by_user_id),
    resolved_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'open'
RETURNING *;

-- name: CloseWarehouseTransferLinesExcept :exec
-- Completes a transfer with missing lines: every other line releases its
-- unit lock; a missing line keeps it until its discrepancy is resolved.
UPDATE warehouse_transfer_lines SET is_open = FALSE
WHERE transfer_id = sqlc.arg(transfer_id) AND NOT (id = ANY(sqlc.arg(keep_ids)::bigint[]));

-- name: CloseWarehouseTransferLine :exec
UPDATE warehouse_transfer_lines SET is_open = FALSE WHERE id = sqlc.arg(id);

-- name: CountOpenWarehouseTransferDiscrepancies :one
SELECT count(*)::bigint FROM warehouse_transfer_discrepancies
WHERE transfer_id = sqlc.arg(transfer_id) AND status = 'open';

-- name: GetWarehouseTransferLine :one
SELECT * FROM warehouse_transfer_lines WHERE id = sqlc.arg(id);
