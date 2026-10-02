-- TEC-156 (F1-02d): projection rebuild, drift scan and repair.
-- Movements are read only (append-only ledger); the repair writes the
-- projections (unit_current_state, fixed_barcode_holdings, units.status /
-- remaining_meters, bin_product_stocks, organization_product_stocks).

-- name: ListRebuildUnitIDs :many
-- Every unit that has a movement or a projection row.
SELECT unit_id FROM stock_movements
UNION
SELECT unit_id FROM unit_current_state
UNION
SELECT unit_id FROM fixed_barcode_holdings
ORDER BY 1;

-- name: ListRebuildUnitIDsByOrganization :many
-- Units that touched the organization: a movement recorded on it or into it
-- as organization/trash owner (every later owner of the unit at that
-- organization follows from such a movement), or a projection row it holds.
SELECT unit_id FROM stock_movements
WHERE organization_id = sqlc.arg(organization_id)
   OR (to_owner_type IN ('organization', 'trash') AND to_owner_id = sqlc.arg(organization_id))
UNION
SELECT unit_id FROM unit_current_state WHERE holder_org_id = sqlc.arg(organization_id)
UNION
SELECT unit_id FROM fixed_barcode_holdings WHERE holder_org_id = sqlc.arg(organization_id)
ORDER BY 1;

-- name: ListUnitsByIDs :many
SELECT * FROM units WHERE id = ANY(sqlc.arg(ids)::bigint[]) ORDER BY id;

-- name: LockUnitsByIDs :many
-- Same first lock as ledger.Post (the unit row), in id order.
SELECT id FROM units WHERE id = ANY(sqlc.arg(ids)::bigint[]) ORDER BY id FOR UPDATE;

-- name: ListStockMovementsByUnitIDs :many
-- Chronological per unit: the unit row lock serialises a unit's movements,
-- so id order is commit order.
SELECT * FROM stock_movements
WHERE unit_id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY unit_id, id;

-- name: ListUnitCurrentStatesByUnitIDs :many
SELECT * FROM unit_current_state
WHERE unit_id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY unit_id;

-- name: ListFixedBarcodeHoldingsByUnitIDs :many
SELECT * FROM fixed_barcode_holdings
WHERE unit_id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY unit_id, id;

-- name: ListBinProductStocksForRebuild :many
SELECT * FROM bin_product_stocks
WHERE sqlc.narg(organization_id)::bigint IS NULL OR organization_id = sqlc.narg(organization_id)::bigint
ORDER BY location_id, product_id;

-- name: ListOrganizationProductStocksForRebuild :many
SELECT * FROM organization_product_stocks
WHERE sqlc.narg(organization_id)::bigint IS NULL OR organization_id = sqlc.narg(organization_id)::bigint
ORDER BY organization_id, product_id;

-- name: UpsertUnitCurrentStateForRepair :exec
INSERT INTO unit_current_state (
    unit_id, brand_id, owner_type, owner_id, holder_org_id, status, last_movement_id
)
VALUES (
    sqlc.arg(unit_id), sqlc.arg(brand_id), sqlc.arg(owner_type), sqlc.arg(owner_id),
    sqlc.arg(holder_org_id), sqlc.arg(status), sqlc.narg(last_movement_id)
)
ON CONFLICT (unit_id) DO UPDATE
SET owner_type = EXCLUDED.owner_type,
    owner_id = EXCLUDED.owner_id,
    holder_org_id = EXCLUDED.holder_org_id,
    status = EXCLUDED.status,
    last_movement_id = EXCLUDED.last_movement_id,
    version = unit_current_state.version + 1;

-- name: DeleteUnitCurrentStateForRepair :exec
DELETE FROM unit_current_state WHERE unit_id = sqlc.arg(unit_id);

-- name: UpsertFixedBarcodeHoldingForRepair :exec
INSERT INTO fixed_barcode_holdings (
    unit_id, brand_id, owner_type, owner_id, holder_org_id, quantity_on_hand, last_movement_id
)
VALUES (
    sqlc.arg(unit_id), sqlc.arg(brand_id), sqlc.arg(owner_type), sqlc.arg(owner_id),
    sqlc.arg(holder_org_id), sqlc.arg(quantity_on_hand), sqlc.narg(last_movement_id)
)
ON CONFLICT (unit_id, owner_type, owner_id) DO UPDATE
SET holder_org_id = EXCLUDED.holder_org_id,
    quantity_on_hand = EXCLUDED.quantity_on_hand,
    last_movement_id = EXCLUDED.last_movement_id,
    version = fixed_barcode_holdings.version + 1;

-- name: DeleteFixedBarcodeHoldingForRepair :exec
DELETE FROM fixed_barcode_holdings WHERE id = sqlc.arg(id);

-- name: SetUnitStatusForRepair :exec
UPDATE units SET status = sqlc.arg(status) WHERE id = sqlc.arg(id);

-- name: SetUnitRemainingMetersForRepair :exec
UPDATE units SET remaining_meters = sqlc.arg(remaining_meters) WHERE id = sqlc.arg(id);

-- name: UpsertBinProductStockForRepair :exec
INSERT INTO bin_product_stocks (location_id, organization_id, brand_id, product_id, quantity, meters)
VALUES (
    sqlc.arg(location_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id),
    sqlc.arg(quantity), sqlc.arg(meters)
)
ON CONFLICT (location_id, product_id) DO UPDATE
SET quantity = EXCLUDED.quantity,
    meters = EXCLUDED.meters;

-- name: UpsertOrganizationProductStockForRepair :exec
INSERT INTO organization_product_stocks (organization_id, product_id, brand_id, quantity, meters)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(product_id), sqlc.arg(brand_id),
    sqlc.arg(quantity), sqlc.arg(meters)
)
ON CONFLICT (organization_id, product_id) DO UPDATE
SET quantity = EXCLUDED.quantity,
    meters = EXCLUDED.meters;
