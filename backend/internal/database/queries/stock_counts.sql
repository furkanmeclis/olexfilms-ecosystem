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
-- TEC-375: list contract (docs/list-contract.md), keys from warehouse
-- usecase CountSort. status sorts by flow rank, warehouse by name.
-- q: note, warehouse name or code.
SELECT c.* FROM stock_counts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR c.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(methods)::text[]), 0) = 0 OR c.method = ANY (sqlc.narg(methods)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(visibilities)::text[]), 0) = 0 OR c.visibility = ANY (sqlc.narg(visibilities)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(scope_types)::text[]), 0) = 0 OR c.scope_type = ANY (sqlc.narg(scope_types)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = c.warehouse_id AND fw.uuid = ANY (sqlc.narg(warehouse_uuids)::uuid[])))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR c.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR c.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR c.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (SELECT 1 FROM warehouses qw
                  WHERE qw.id = c.warehouse_id
                    AND (qw.name ILIKE '%' || sqlc.narg(q)::text || '%' OR qw.code ILIKE '%' || sqlc.narg(q)::text || '%')))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN c.created_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN c.created_at END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'status' THEN
    CASE c.status WHEN 'draft' THEN 1 WHEN 'in_progress' THEN 2 WHEN 'pending_review' THEN 3
                  WHEN 'approved' THEN 4 ELSE 5 END END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'status' THEN
    CASE c.status WHEN 'draft' THEN 1 WHEN 'in_progress' THEN 2 WHEN 'pending_review' THEN 3
                  WHEN 'approved' THEN 4 ELSE 5 END END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'warehouse' THEN
    (SELECT sw.name FROM warehouses sw WHERE sw.id = c.warehouse_id) END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'warehouse' THEN
    (SELECT sw.name FROM warehouses sw WHERE sw.id = c.warehouse_id) END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN c.id END DESC,
  c.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountStockCounts :one
SELECT COUNT(*) FROM stock_counts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR c.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(methods)::text[]), 0) = 0 OR c.method = ANY (sqlc.narg(methods)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(visibilities)::text[]), 0) = 0 OR c.visibility = ANY (sqlc.narg(visibilities)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(scope_types)::text[]), 0) = 0 OR c.scope_type = ANY (sqlc.narg(scope_types)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = c.warehouse_id AND fw.uuid = ANY (sqlc.narg(warehouse_uuids)::uuid[])))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR c.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR c.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR c.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (SELECT 1 FROM warehouses qw
                  WHERE qw.id = c.warehouse_id
                    AND (qw.name ILIKE '%' || sqlc.narg(q)::text || '%' OR qw.code ILIKE '%' || sqlc.narg(q)::text || '%')));

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

-- name: ListStockCountScansPage :many
-- TEC-375: one page of a count's scans (scan order).
SELECT * FROM stock_count_scans WHERE count_id = sqlc.arg(count_id) ORDER BY id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountStockCountScans :one
SELECT COUNT(*)::bigint FROM stock_count_scans WHERE count_id = sqlc.arg(count_id);

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
