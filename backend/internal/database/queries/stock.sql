-- TEC-153: stock ledger primitives for ledger.Post (TEC-154) and the stock
-- API. Every write goes through ledger.Post inside one transaction: lock the
-- state row (FOR UPDATE), append the movement, update the projections.
-- Brand filters follow K1; the warehouse side is brand-independent (K20), so
-- holder/organization filters are applied by the use case scope.

-- ---------------------------------------------------------------------------
-- Warehouse locations (minimal; TEC-95 extends).

-- name: CreateWarehouseLocation :one
INSERT INTO warehouse_locations (organization_id, parent_id, code, name, active)
VALUES (
    sqlc.arg(organization_id), sqlc.narg(parent_id), sqlc.arg(code),
    sqlc.arg(name), sqlc.arg(active)
)
RETURNING *;

-- name: GetWarehouseLocation :one
SELECT * FROM warehouse_locations
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: GetWarehouseLocationByUUID :one
SELECT * FROM warehouse_locations WHERE uuid = sqlc.arg(uuid);

-- name: ListWarehouseLocations :many
SELECT * FROM warehouse_locations
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
ORDER BY code, id;

-- name: UpdateWarehouseLocation :one
UPDATE warehouse_locations
SET parent_id = sqlc.narg(parent_id),
    code = sqlc.arg(code),
    name = sqlc.arg(name),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Units.

-- name: CreateUnit :one
INSERT INTO units (
    organization_id, brand_id, product_id, barcode, unit_kind, source, status,
    initial_meters, remaining_meters, connection_id, external_id, external_status
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(barcode),
    sqlc.arg(unit_kind), sqlc.arg(source), sqlc.arg(status),
    sqlc.narg(initial_meters), sqlc.narg(remaining_meters),
    sqlc.narg(connection_id), sqlc.narg(external_id), sqlc.narg(external_status)
)
RETURNING *;

-- name: GetUnit :one
SELECT * FROM units WHERE id = sqlc.arg(id);

-- name: GetUnitByUUID :one
SELECT * FROM units WHERE uuid = sqlc.arg(uuid);

-- name: GetUnitByBarcode :one
SELECT * FROM units
WHERE brand_id = sqlc.arg(brand_id) AND barcode = sqlc.arg(barcode);

-- name: ListUnitsByBarcode :many
-- Brand-independent barcode lookup for the warehouse scanner (K20): the
-- caller narrows the result by scope.
SELECT * FROM units WHERE barcode = sqlc.arg(barcode) ORDER BY brand_id, id;

-- name: LockUnit :one
-- Locks the unit row for meters/status/product changes in ledger.Post.
SELECT * FROM units WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: UpdateUnitStatus :one
UPDATE units SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUnitRemainingMeters :one
-- The CHECK constraint rejects a negative or above-initial value.
UPDATE units SET remaining_meters = sqlc.arg(remaining_meters)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUnitProduct :one
-- Reclassification only (stock_reclassifications approval).
UPDATE units SET product_id = sqlc.arg(product_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUnitExternal :one
UPDATE units
SET connection_id = sqlc.narg(connection_id),
    external_id = sqlc.narg(external_id),
    external_status = sqlc.narg(external_status)
WHERE id = sqlc.arg(id)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Stock movements (append-only: insert and read only).

-- name: InsertStockMovement :one
-- Idempotent append: a repeated idempotency_key inserts nothing and returns
-- no row (pgx.ErrNoRows); the caller then reads GetStockMovementByIdempotencyKey.
INSERT INTO stock_movements (
    organization_id, brand_id, unit_id, product_id, type,
    quantity_delta, meters_delta,
    from_owner_type, from_owner_id, to_owner_type, to_owner_id,
    from_status, to_status, reference_type, reference_id,
    actor_user_id, reason, metadata, idempotency_key
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(unit_id), sqlc.arg(product_id), sqlc.arg(type),
    sqlc.arg(quantity_delta), sqlc.arg(meters_delta),
    sqlc.narg(from_owner_type), sqlc.narg(from_owner_id), sqlc.narg(to_owner_type), sqlc.narg(to_owner_id),
    sqlc.narg(from_status), sqlc.narg(to_status), sqlc.narg(reference_type), sqlc.narg(reference_id),
    sqlc.narg(actor_user_id), sqlc.narg(reason), sqlc.arg(metadata), sqlc.arg(idempotency_key)
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: GetStockMovement :one
SELECT * FROM stock_movements WHERE id = sqlc.arg(id);

-- name: GetStockMovementByIdempotencyKey :one
SELECT * FROM stock_movements WHERE idempotency_key = sqlc.arg(idempotency_key);

-- name: ListStockMovementsByUnit :many
-- Barcode history in ledger order.
SELECT * FROM stock_movements
WHERE unit_id = sqlc.arg(unit_id)
ORDER BY created_at, id;

-- name: ListStockMovementsByReference :many
SELECT * FROM stock_movements
WHERE reference_type = sqlc.arg(reference_type) AND reference_id = sqlc.arg(reference_id)
ORDER BY created_at, id;

-- name: ListStockMovementsByOrganization :many
SELECT * FROM stock_movements
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(before_id)::bigint IS NULL OR id < sqlc.narg(before_id)::bigint)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit);

-- ---------------------------------------------------------------------------
-- Unit current state (serial units; one active owner).

-- name: InsertUnitCurrentState :one
INSERT INTO unit_current_state (
    unit_id, brand_id, owner_type, owner_id, holder_org_id, status, last_movement_id
)
VALUES (
    sqlc.arg(unit_id), sqlc.arg(brand_id), sqlc.arg(owner_type), sqlc.arg(owner_id),
    sqlc.arg(holder_org_id), sqlc.arg(status), sqlc.narg(last_movement_id)
)
RETURNING *;

-- name: GetUnitCurrentState :one
SELECT * FROM unit_current_state WHERE unit_id = sqlc.arg(unit_id);

-- name: LockUnitCurrentState :one
-- Serialises every ledger write on one unit (double owner / double
-- consumption guard).
SELECT * FROM unit_current_state WHERE unit_id = sqlc.arg(unit_id) FOR UPDATE;

-- name: UpdateUnitCurrentState :one
-- Optimistic check on version in addition to the row lock.
UPDATE unit_current_state
SET owner_type = sqlc.arg(owner_type),
    owner_id = sqlc.arg(owner_id),
    holder_org_id = sqlc.arg(holder_org_id),
    status = sqlc.arg(status),
    last_movement_id = sqlc.arg(last_movement_id),
    version = version + 1
WHERE unit_id = sqlc.arg(unit_id) AND version = sqlc.arg(version)
RETURNING *;

-- name: ListUnitCurrentStatesByHolder :many
SELECT s.*, u.barcode, u.product_id, u.remaining_meters
FROM unit_current_state s
JOIN units u ON u.id = s.unit_id
WHERE s.holder_org_id = sqlc.arg(holder_org_id)
  AND (sqlc.narg(status)::text IS NULL OR s.status = sqlc.narg(status)::text)
ORDER BY s.unit_id;

-- name: DeleteAllUnitCurrentStates :execrows
-- Rebuild only (TEC-94d): the projection is regenerated from the ledger.
DELETE FROM unit_current_state;

-- ---------------------------------------------------------------------------
-- Fixed barcode holdings (quantity per unit and owner).

-- name: LockFixedBarcodeHolding :one
SELECT * FROM fixed_barcode_holdings
WHERE unit_id = sqlc.arg(unit_id) AND owner_type = sqlc.arg(owner_type) AND owner_id = sqlc.arg(owner_id)
FOR UPDATE;

-- name: LockFixedBarcodeHoldingsByUnit :many
SELECT * FROM fixed_barcode_holdings
WHERE unit_id = sqlc.arg(unit_id)
ORDER BY id
FOR UPDATE;

-- Projection deltas are two steps: Ensure* creates a zero row on first use
-- (no-op otherwise), Add* applies the signed delta. A plain UPDATE keeps the
-- CHECK (>= 0) on the result only; an INSERT ... ON CONFLICT DO UPDATE would
-- check the proposed row and reject every negative delta.

-- name: EnsureFixedBarcodeHolding :exec
INSERT INTO fixed_barcode_holdings (unit_id, brand_id, owner_type, owner_id, holder_org_id)
VALUES (
    sqlc.arg(unit_id), sqlc.arg(brand_id), sqlc.arg(owner_type), sqlc.arg(owner_id),
    sqlc.arg(holder_org_id)
)
ON CONFLICT (unit_id, owner_type, owner_id) DO NOTHING;

-- name: AddFixedBarcodeHolding :one
-- The CHECK rejects a negative result.
UPDATE fixed_barcode_holdings
SET quantity_on_hand = quantity_on_hand + sqlc.arg(quantity_delta)::int,
    last_movement_id = sqlc.arg(last_movement_id),
    version = version + 1
WHERE unit_id = sqlc.arg(unit_id) AND owner_type = sqlc.arg(owner_type) AND owner_id = sqlc.arg(owner_id)
RETURNING *;

-- name: ListFixedBarcodeHoldingsByUnit :many
SELECT * FROM fixed_barcode_holdings WHERE unit_id = sqlc.arg(unit_id) ORDER BY id;

-- name: ListFixedBarcodeHoldingsByHolder :many
SELECT * FROM fixed_barcode_holdings
WHERE holder_org_id = sqlc.arg(holder_org_id) AND quantity_on_hand > 0
ORDER BY unit_id, id;

-- name: DeleteAllFixedBarcodeHoldings :execrows
-- Rebuild only (TEC-94d).
DELETE FROM fixed_barcode_holdings;

-- ---------------------------------------------------------------------------
-- Product stock projections.

-- name: EnsureBinProductStock :exec
INSERT INTO bin_product_stocks (location_id, organization_id, brand_id, product_id)
VALUES (sqlc.arg(location_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id))
ON CONFLICT (location_id, product_id) DO NOTHING;

-- name: AddBinProductStock :one
-- Signed deltas; the CHECK rejects negative stock.
UPDATE bin_product_stocks
SET quantity = quantity + sqlc.arg(quantity_delta)::int,
    meters = meters + sqlc.arg(meters_delta)::numeric
WHERE location_id = sqlc.arg(location_id) AND product_id = sqlc.arg(product_id)
RETURNING *;

-- name: LockBinProductStock :one
SELECT * FROM bin_product_stocks
WHERE location_id = sqlc.arg(location_id) AND product_id = sqlc.arg(product_id)
FOR UPDATE;

-- name: ListBinProductStocksByLocation :many
SELECT * FROM bin_product_stocks
WHERE location_id = sqlc.arg(location_id)
ORDER BY product_id;

-- name: ListBinProductStocksByOrganization :many
SELECT * FROM bin_product_stocks
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY location_id, product_id;

-- name: DeleteAllBinProductStocks :execrows
-- Rebuild only (TEC-94d).
DELETE FROM bin_product_stocks;

-- name: EnsureOrganizationProductStock :exec
INSERT INTO organization_product_stocks (organization_id, product_id, brand_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(product_id), sqlc.arg(brand_id))
ON CONFLICT (organization_id, product_id) DO NOTHING;

-- name: AddOrganizationProductStock :one
-- Signed deltas; the CHECK rejects negative stock.
UPDATE organization_product_stocks
SET quantity = quantity + sqlc.arg(quantity_delta)::int,
    meters = meters + sqlc.arg(meters_delta)::numeric
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id)
RETURNING *;

-- name: LockOrganizationProductStock :one
SELECT * FROM organization_product_stocks
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id)
FOR UPDATE;

-- name: ListOrganizationProductStocks :many
SELECT * FROM organization_product_stocks
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id)::bigint)
ORDER BY product_id;

-- name: DeleteAllOrganizationProductStocks :execrows
-- Rebuild only (TEC-94d).
DELETE FROM organization_product_stocks;

-- ---------------------------------------------------------------------------
-- Reclassification requests.

-- name: CreateStockReclassification :one
INSERT INTO stock_reclassifications (
    organization_id, brand_id, unit_id, from_product_id, to_product_id, reason, requested_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(unit_id), sqlc.arg(from_product_id),
    sqlc.arg(to_product_id), sqlc.arg(reason), sqlc.narg(requested_by_user_id)
)
RETURNING *;

-- name: GetStockReclassification :one
SELECT * FROM stock_reclassifications
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: LockStockReclassification :one
SELECT * FROM stock_reclassifications
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: DecideStockReclassification :one
UPDATE stock_reclassifications
SET status = sqlc.arg(status),
    decided_by_user_id = sqlc.narg(decided_by_user_id),
    decided_at = NOW(),
    decision_note = sqlc.narg(decision_note),
    movement_id = sqlc.narg(movement_id)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: ListStockReclassifications :many
SELECT * FROM stock_reclassifications
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC;

-- ---------------------------------------------------------------------------
-- Stock import staging.

-- name: CreateStockImportBatch :one
INSERT INTO stock_import_batches (
    organization_id, brand_id, created_by_user_id, source_filename, target_owner_type, target_owner_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(created_by_user_id),
    sqlc.narg(source_filename), sqlc.narg(target_owner_type), sqlc.narg(target_owner_id)
)
RETURNING *;

-- name: GetStockImportBatch :one
SELECT * FROM stock_import_batches
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: LockStockImportBatch :one
SELECT * FROM stock_import_batches WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: UpdateStockImportBatchStatus :one
UPDATE stock_import_batches
SET status = sqlc.arg(status),
    rows_total = sqlc.arg(rows_total),
    rows_new = sqlc.arg(rows_new),
    rows_duplicate = sqlc.arg(rows_duplicate),
    rows_invalid = sqlc.arg(rows_invalid),
    rows_conflict = sqlc.arg(rows_conflict),
    error = sqlc.narg(error),
    applied_at = CASE WHEN sqlc.arg(status)::text = 'applied' THEN NOW() ELSE applied_at END,
    undone_at = CASE WHEN sqlc.arg(status)::text = 'undone' THEN NOW() ELSE undone_at END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListStockImportBatches :many
SELECT * FROM stock_import_batches
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY created_at DESC, id DESC;

-- name: InsertStockImportRow :one
INSERT INTO stock_import_rows (
    batch_id, row_number, barcode, product_sku, product_id, quantity, meters,
    target_owner_type, target_owner_id, raw, row_status, errors
)
VALUES (
    sqlc.arg(batch_id), sqlc.arg(row_number), sqlc.narg(barcode), sqlc.narg(product_sku),
    sqlc.narg(product_id), sqlc.narg(quantity), sqlc.narg(meters),
    sqlc.narg(target_owner_type), sqlc.narg(target_owner_id), sqlc.arg(raw),
    sqlc.arg(row_status), sqlc.arg(errors)
)
RETURNING *;

-- name: ListStockImportRows :many
SELECT * FROM stock_import_rows
WHERE batch_id = sqlc.arg(batch_id)
  AND (sqlc.narg(row_status)::text IS NULL OR row_status = sqlc.narg(row_status)::text)
ORDER BY row_number;

-- name: UpdateStockImportRowResult :one
UPDATE stock_import_rows
SET row_status = sqlc.arg(row_status),
    errors = sqlc.arg(errors),
    unit_id = sqlc.narg(unit_id),
    movement_id = sqlc.narg(movement_id)
WHERE id = sqlc.arg(id)
RETURNING *;
