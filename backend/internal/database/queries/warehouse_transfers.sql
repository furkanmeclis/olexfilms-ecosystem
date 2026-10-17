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
-- TEC-375: list contract (docs/list-contract.md), keys from warehouse
-- usecase TransferSort. status sorts by flow rank. q: transfer no, note.
SELECT t.* FROM warehouse_transfers t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR t.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(from_warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = t.from_warehouse_id AND fw.uuid = ANY (sqlc.narg(from_warehouse_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(to_warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses tw
                  WHERE tw.id = t.to_warehouse_id AND tw.uuid = ANY (sqlc.narg(to_warehouse_uuids)::uuid[])))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR t.transfer_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.note ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'transfer_no' THEN t.transfer_no END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'transfer_no' THEN t.transfer_no END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'status' THEN
    CASE t.status WHEN 'draft' THEN 1 WHEN 'in_transit' THEN 2 WHEN 'completed' THEN 3 ELSE 4 END END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'status' THEN
    CASE t.status WHEN 'draft' THEN 1 WHEN 'in_transit' THEN 2 WHEN 'completed' THEN 3 ELSE 4 END END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN t.created_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN t.created_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWarehouseTransfers :one
SELECT count(*) FROM warehouse_transfers t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR t.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(from_warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = t.from_warehouse_id AND fw.uuid = ANY (sqlc.narg(from_warehouse_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(to_warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses tw
                  WHERE tw.id = t.to_warehouse_id AND tw.uuid = ANY (sqlc.narg(to_warehouse_uuids)::uuid[])))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR t.transfer_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.note ILIKE '%' || sqlc.narg(q)::text || '%');

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
INSERT INTO warehouse_transfer_lines (transfer_id, unit_id, target_location_id, source_location_id, quantity)
VALUES (sqlc.arg(transfer_id), sqlc.arg(unit_id), sqlc.narg(target_location_id), sqlc.narg(source_location_id), sqlc.arg(quantity))
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

-- name: ListOrderPlacementUnits :many
-- The units of an order with whether each is still on hand unplaced in the
-- organization (available, owned by the organization itself, TEC-765).
SELECT u.uuid AS unit_uuid, u.barcode, u.unit_kind, u.remaining_meters,
       oiu.quantity, oiu.meters,
       p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name, p.unit_type AS product_unit_type,
       COALESCE(s.holder_org_id = sqlc.arg(organization_id)::bigint
                AND s.status = 'available' AND s.owner_type = 'organization', FALSE)::boolean AS unplaced
FROM order_item_units oiu
JOIN order_items i ON i.id = oiu.order_item_id
JOIN units u ON u.id = oiu.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN unit_current_state s ON s.unit_id = u.id
WHERE i.order_id = sqlc.arg(order_id)
ORDER BY oiu.order_item_id, oiu.id;
