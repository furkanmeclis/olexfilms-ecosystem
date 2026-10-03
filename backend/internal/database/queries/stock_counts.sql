-- TEC-206: stock counts (000068). Every query is bound to one organization;
-- the warehouse side is brand-independent (K20).

-- name: CreateStockCount :one
INSERT INTO stock_counts (
    organization_id, brand_id, warehouse_id, method, visibility, scope_type,
    scope_room_id, scope_location_id, scope_product_id, note, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(warehouse_id), sqlc.arg(method),
    sqlc.arg(visibility), sqlc.arg(scope_type), sqlc.narg(scope_room_id), sqlc.narg(scope_location_id),
    sqlc.narg(scope_product_id), sqlc.narg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetStockCountByUUID :one
SELECT * FROM stock_counts
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: LockStockCountByUUID :one
SELECT * FROM stock_counts
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: ListStockCounts :many
SELECT * FROM stock_counts
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountStockCounts :one
SELECT COUNT(*) FROM stock_counts
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: ApproveStockCountStart :one
UPDATE stock_counts
SET start_approved_at = NOW(), start_approved_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: StartStockCount :one
UPDATE stock_counts
SET status = 'in_progress', started_at = NOW(), started_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CompleteStockCount :one
UPDATE stock_counts
SET status = 'pending_review', completed_at = NOW(), completed_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ApproveStockCount :one
UPDATE stock_counts
SET status = 'approved', approved_at = NOW(), approved_by_user_id = sqlc.narg(user_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CancelStockCount :one
UPDATE stock_counts
SET status = 'cancelled', cancelled_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Scope.

-- name: ListWarehouseScopeLocationIDs :many
-- Typed locations of a warehouse, optionally one room.
SELECT id FROM warehouse_locations
WHERE organization_id = sqlc.arg(organization_id)
  AND warehouse_id = sqlc.arg(warehouse_id)
  AND (sqlc.narg(room_id)::bigint IS NULL OR room_id = sqlc.narg(room_id)::bigint);

-- name: ListLocationSubtreeIDs :many
-- A location and everything below it.
WITH RECURSIVE sub AS (
    SELECT wl.id FROM warehouse_locations wl
    WHERE wl.id = sqlc.arg(root_id) AND wl.organization_id = sqlc.arg(organization_id)
    UNION
    SELECT c.id FROM warehouse_locations c JOIN sub ON c.parent_id = sub.id
    WHERE c.organization_id = sqlc.arg(organization_id)
)
SELECT id FROM sub;

-- ---------------------------------------------------------------------------
-- Expected sets (computed at completion and for guided counts).

-- name: ListCountExpectedSerial :many
-- Serial units on hand at the given locations of the organization.
SELECT s.unit_id, s.owner_id AS location_id, u.product_id, u.remaining_meters
FROM unit_current_state s
JOIN units u ON u.id = s.unit_id
WHERE s.holder_org_id = sqlc.arg(organization_id)
  AND s.owner_type = 'warehouse_location'
  AND s.owner_id = ANY(sqlc.arg(location_ids)::bigint[])
  AND s.status IN ('available', 'placed')
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
ORDER BY s.unit_id;

-- name: ListCountExpectedUnlocatedSerial :many
-- Serial units on hand held by the organization itself (no location).
SELECT s.unit_id, u.product_id, u.remaining_meters
FROM unit_current_state s
JOIN units u ON u.id = s.unit_id
WHERE s.holder_org_id = sqlc.arg(organization_id)
  AND s.owner_type = 'organization'
  AND s.owner_id = sqlc.arg(organization_id)
  AND s.status IN ('available', 'placed')
ORDER BY s.unit_id;

-- name: ListCountExpectedFixed :many
-- Fixed barcode holdings at the given locations of the organization.
SELECT h.unit_id, h.owner_id AS location_id, h.quantity_on_hand, u.product_id
FROM fixed_barcode_holdings h
JOIN units u ON u.id = h.unit_id
WHERE h.holder_org_id = sqlc.arg(organization_id)
  AND h.owner_type = 'warehouse_location'
  AND h.owner_id = ANY(sqlc.arg(location_ids)::bigint[])
  AND h.quantity_on_hand > 0
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
ORDER BY h.unit_id, h.owner_id;

-- name: ListCountExpectedUnlocatedFixed :many
-- Fixed barcode holdings of the organization itself (no location).
SELECT h.unit_id, h.quantity_on_hand, u.product_id
FROM fixed_barcode_holdings h
JOIN units u ON u.id = h.unit_id
WHERE h.holder_org_id = sqlc.arg(organization_id)
  AND h.owner_type = 'organization'
  AND h.owner_id = sqlc.arg(organization_id)
  AND h.quantity_on_hand > 0
ORDER BY h.unit_id;

-- name: GetFixedHoldingQuantity :one
SELECT COALESCE((
    SELECT quantity_on_hand FROM fixed_barcode_holdings
    WHERE unit_id = sqlc.arg(unit_id) AND owner_type = sqlc.arg(owner_type) AND owner_id = sqlc.arg(owner_id)
), 0)::int AS quantity;

-- ---------------------------------------------------------------------------
-- Scans.

-- name: InsertStockCountScan :one
INSERT INTO stock_count_scans (
    count_id, kind, raw_code, unit_id, product_id, location_id, quantity, meters, scanned_by_user_id
)
VALUES (
    sqlc.arg(count_id), sqlc.arg(kind), sqlc.arg(raw_code), sqlc.narg(unit_id), sqlc.narg(product_id),
    sqlc.narg(location_id), sqlc.arg(quantity), sqlc.narg(meters), sqlc.narg(scanned_by_user_id)
)
RETURNING *;

-- name: ListStockCountScans :many
SELECT * FROM stock_count_scans WHERE count_id = sqlc.arg(count_id) ORDER BY id;

-- name: GetStockCountScanByUUID :one
SELECT * FROM stock_count_scans WHERE uuid = sqlc.arg(uuid) AND count_id = sqlc.arg(count_id);

-- name: DeleteStockCountScan :execrows
DELETE FROM stock_count_scans WHERE id = sqlc.arg(id) AND count_id = sqlc.arg(count_id);

-- name: StockCountSerialScanExists :one
SELECT EXISTS (
    SELECT 1 FROM stock_count_scans
    WHERE count_id = sqlc.arg(count_id) AND kind = 'serial' AND unit_id = sqlc.arg(unit_id)
);

-- name: LastStockCountLocationScan :one
-- The scanning user's current location context (location_first).
SELECT location_id FROM stock_count_scans
WHERE count_id = sqlc.arg(count_id) AND kind = 'location'
  AND scanned_by_user_id IS NOT DISTINCT FROM sqlc.narg(user_id)::bigint
ORDER BY id DESC
LIMIT 1;

-- ---------------------------------------------------------------------------
-- Lines.

-- name: InsertStockCountLine :one
INSERT INTO stock_count_lines (
    count_id, line_kind, unit_id, product_id, expected_location_id, counted_location_id,
    expected_quantity, counted_quantity, expected_meters, counted_meters, result
)
VALUES (
    sqlc.arg(count_id), sqlc.arg(line_kind), sqlc.narg(unit_id), sqlc.arg(product_id),
    sqlc.narg(expected_location_id), sqlc.narg(counted_location_id), sqlc.arg(expected_quantity),
    sqlc.arg(counted_quantity), sqlc.narg(expected_meters), sqlc.narg(counted_meters), sqlc.arg(result)
)
RETURNING *;

-- name: ListStockCountLines :many
SELECT * FROM stock_count_lines WHERE count_id = sqlc.arg(count_id) ORDER BY id;

-- name: ResolveStockCountLine :one
UPDATE stock_count_lines
SET resolution = sqlc.arg(resolution), note = sqlc.narg(note), resolved_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListStockCountUnits :many
SELECT id, uuid, barcode, unit_kind, product_id FROM units WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListStockCountProducts :many
SELECT id, uuid, sku, name FROM products WHERE id = ANY(sqlc.arg(ids)::bigint[]);
