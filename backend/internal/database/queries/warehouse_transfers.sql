-- TEC-205: warehouse -> warehouse transfer documents (draft -> in_transit ->
-- completed / cancelled) inside one organization.

-- name: CreateWarehouseTransfer :one
INSERT INTO warehouse_transfers (
    organization_id, brand_id, from_warehouse_id, to_warehouse_id, to_location_id, note, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(from_warehouse_id), sqlc.arg(to_warehouse_id),
    sqlc.narg(to_location_id), sqlc.narg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetWarehouseTransferByUUID :one
SELECT * FROM warehouse_transfers
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: LockWarehouseTransferByUUID :one
SELECT * FROM warehouse_transfers
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: ListWarehouseTransfers :many
SELECT * FROM warehouse_transfers
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWarehouseTransfers :one
SELECT count(*) FROM warehouse_transfers
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: ShipWarehouseTransfer :one
UPDATE warehouse_transfers
SET status = 'in_transit', shipped_at = NOW(), shipped_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: CompleteWarehouseTransfer :one
UPDATE warehouse_transfers
SET status = 'completed', completed_at = NOW(), completed_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id) AND status = 'in_transit'
RETURNING *;

-- name: CancelWarehouseTransfer :one
UPDATE warehouse_transfers
SET status = 'cancelled', cancelled_at = NOW(), cancelled_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id) AND status IN ('draft', 'in_transit')
RETURNING *;

-- name: InsertWarehouseTransferLine :one
INSERT INTO warehouse_transfer_lines (transfer_id, unit_id, target_location_id)
VALUES (sqlc.arg(transfer_id), sqlc.arg(unit_id), sqlc.narg(target_location_id))
RETURNING *;

-- name: ListWarehouseTransferLines :many
SELECT * FROM warehouse_transfer_lines WHERE transfer_id = sqlc.arg(transfer_id) ORDER BY id;

-- name: CountWarehouseTransferLines :one
SELECT count(*) FROM warehouse_transfer_lines WHERE transfer_id = sqlc.arg(transfer_id);

-- name: DeleteWarehouseTransferLine :execrows
DELETE FROM warehouse_transfer_lines
WHERE uuid = sqlc.arg(uuid) AND transfer_id = sqlc.arg(transfer_id);

-- name: SetWarehouseTransferLineTarget :execrows
UPDATE warehouse_transfer_lines
SET target_location_id = sqlc.arg(location_id)
WHERE transfer_id = sqlc.arg(transfer_id) AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: SetWarehouseTransferLineOut :exec
UPDATE warehouse_transfer_lines
SET out_movement_id = sqlc.arg(movement_id), source_location_id = sqlc.narg(source_location_id)
WHERE id = sqlc.arg(id);

-- name: SetWarehouseTransferLineIn :exec
UPDATE warehouse_transfer_lines
SET in_movement_id = sqlc.arg(in_movement_id), placement_movement_id = sqlc.arg(placement_movement_id),
    target_location_id = sqlc.arg(target_location_id)
WHERE id = sqlc.arg(id);

-- name: SetWarehouseTransferLineRestore :exec
UPDATE warehouse_transfer_lines
SET restore_movement_id = sqlc.arg(movement_id)
WHERE id = sqlc.arg(id);

-- name: CloseWarehouseTransferLines :exec
-- Releases the unit lock when the transfer is completed or cancelled.
UPDATE warehouse_transfer_lines SET is_open = FALSE WHERE transfer_id = sqlc.arg(transfer_id);

-- name: FindOpenWarehouseTransferForUnit :one
-- The open (draft or in_transit) warehouse transfer holding the unit.
SELECT t.id, t.uuid, t.transfer_no, t.status
FROM warehouse_transfer_lines l
JOIN warehouse_transfers t ON t.id = l.transfer_id
WHERE l.unit_id = sqlc.arg(unit_id) AND l.is_open
LIMIT 1;

-- name: CountOpenWarehouseTransferLinesByUnit :one
-- Used by other flows (stock transfer requests) to respect the lock.
SELECT count(*) FROM warehouse_transfer_lines WHERE unit_id = sqlc.arg(unit_id) AND is_open;
