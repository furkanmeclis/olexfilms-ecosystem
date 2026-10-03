-- TEC-257: migrator step 5 (warehouse structure, stock units and their
-- initial ownership). Written only by cmd/migrator inside a step
-- transaction. The ownership rows carry no movement (last_movement_id NULL);
-- F2-01g opens the ledger and rebuilds the projections from it.

-- name: MigratorFindWarehouse :one
SELECT uuid FROM warehouses WHERE organization_id = sqlc.arg(organization_id) AND code = sqlc.arg(code);

-- name: MigratorWarehouseIDByUUID :one
SELECT id FROM warehouses WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: MigratorInsertWarehouse :one
INSERT INTO warehouses (uuid, organization_id, code, name, address, active, sort_order, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.narg(address),
    sqlc.arg(active), sqlc.arg(sort_order), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateWarehouse :exec
UPDATE warehouses
SET code = sqlc.arg(code), name = sqlc.arg(name), address = sqlc.narg(address),
    active = sqlc.arg(active), sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: MigratorFindRoom :one
SELECT uuid FROM rooms WHERE warehouse_id = sqlc.arg(warehouse_id) AND code = sqlc.arg(code);

-- name: MigratorRoomByUUID :one
SELECT id, warehouse_id FROM rooms WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: MigratorInsertRoom :one
INSERT INTO rooms (uuid, organization_id, warehouse_id, code, name, active, sort_order, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(warehouse_id), sqlc.arg(code), sqlc.arg(name),
    sqlc.arg(active), sqlc.arg(sort_order), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateRoom :exec
UPDATE rooms
SET code = sqlc.arg(code), name = sqlc.arg(name), active = sqlc.arg(active), sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: MigratorRoomFullCodePrefix :one
-- The full_code prefix of the room's root locations (<warehouse>-<room>).
SELECT (w.code || '-' || r.code)::text AS prefix
FROM rooms r JOIN warehouses w ON w.id = r.warehouse_id
WHERE r.id = sqlc.arg(id) AND r.organization_id = sqlc.arg(organization_id);

-- name: MigratorFindLocation :one
-- A typed location of the organization with the full code (rooms, aisles,
-- shelves and bins are unique by full_code).
SELECT uuid FROM warehouse_locations
WHERE organization_id = sqlc.arg(organization_id) AND full_code = sqlc.arg(full_code)::text;

-- name: MigratorLocationByUUID :one
SELECT id, room_id, full_code FROM warehouse_locations
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: MigratorInsertLocation :one
-- warehouse_id and full_code are derived by trg_warehouse_locations_derive.
INSERT INTO warehouse_locations (uuid, organization_id, room_id, parent_id, type, code, name, active, sort_order, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(room_id), sqlc.narg(parent_id), sqlc.arg(type),
    sqlc.arg(code), sqlc.arg(name), sqlc.arg(active), sqlc.arg(sort_order),
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id, full_code;

-- name: MigratorUpdateLocation :exec
UPDATE warehouse_locations
SET room_id = sqlc.arg(room_id), parent_id = sqlc.narg(parent_id), type = sqlc.arg(type),
    code = sqlc.arg(code), name = sqlc.arg(name), active = sqlc.arg(active), sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: MigratorProductForUnit :one
SELECT id, uses_fixed_barcode, unit_type FROM products
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorFindUnitByBarcode :one
SELECT uuid FROM units WHERE brand_id = sqlc.arg(brand_id) AND barcode = sqlc.arg(barcode);

-- name: MigratorUnitByUUID :one
SELECT id, product_id, unit_kind, status FROM units
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorInsertUnit :one
INSERT INTO units (uuid, organization_id, brand_id, product_id, barcode, unit_kind, source, status, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(barcode),
    sqlc.arg(unit_kind), 'imported', sqlc.arg(status), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateUnit :exec
-- Product and status only: issuer, brand, barcode and kind are immutable.
UPDATE units SET product_id = sqlc.arg(product_id), status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorUnitHasMovements :one
SELECT EXISTS (SELECT 1 FROM stock_movements WHERE unit_id = sqlc.arg(unit_id)::bigint);

-- name: MigratorUpsertUnitState :execrows
-- Initial ownership of a serial unit. A state that already follows a
-- movement is the ledger's and is left alone (0 rows).
INSERT INTO unit_current_state (unit_id, brand_id, owner_type, owner_id, holder_org_id, status)
VALUES (
    sqlc.arg(unit_id), sqlc.arg(brand_id), sqlc.arg(owner_type), sqlc.arg(owner_id),
    sqlc.arg(holder_org_id), sqlc.arg(status)
)
ON CONFLICT (unit_id) DO UPDATE
SET owner_type = EXCLUDED.owner_type, owner_id = EXCLUDED.owner_id,
    holder_org_id = EXCLUDED.holder_org_id, status = EXCLUDED.status,
    version = unit_current_state.version + 1
WHERE unit_current_state.last_movement_id IS NULL;

-- name: MigratorDeleteUnitState :exec
DELETE FROM unit_current_state WHERE unit_id = sqlc.arg(unit_id) AND last_movement_id IS NULL;

-- name: MigratorDeleteFixedHoldings :exec
DELETE FROM fixed_barcode_holdings WHERE unit_id = sqlc.arg(unit_id) AND last_movement_id IS NULL;

-- name: MigratorInsertFixedHolding :exec
INSERT INTO fixed_barcode_holdings (unit_id, brand_id, owner_type, owner_id, holder_org_id, quantity_on_hand)
VALUES (
    sqlc.arg(unit_id), sqlc.arg(brand_id), sqlc.arg(owner_type), sqlc.arg(owner_id),
    sqlc.arg(holder_org_id), sqlc.arg(quantity_on_hand)
);

-- name: MigratorLocationHasMovements :one
SELECT EXISTS (
    SELECT 1 FROM stock_movements
    WHERE (from_owner_type = 'warehouse_location' AND from_owner_id = sqlc.arg(location_id)::bigint)
       OR (to_owner_type = 'warehouse_location' AND to_owner_id = sqlc.arg(location_id)::bigint)
);

-- name: MigratorGetBinStock :one
SELECT quantity FROM bin_product_stocks
WHERE location_id = sqlc.arg(location_id) AND product_id = sqlc.arg(product_id);

-- name: MigratorUpsertBinStock :exec
INSERT INTO bin_product_stocks (location_id, organization_id, brand_id, product_id, quantity)
VALUES (sqlc.arg(location_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(quantity))
ON CONFLICT (location_id, product_id) DO UPDATE SET quantity = EXCLUDED.quantity;

-- name: MigratorCountLocationUnits :one
-- Pieces of the product the migrated ownership puts at the location: serial
-- units plus fixed barcode quantities.
SELECT (
    (SELECT COUNT(*) FROM unit_current_state s JOIN units u ON u.id = s.unit_id
     WHERE s.owner_location_id = sqlc.arg(location_id)::bigint AND u.product_id = sqlc.arg(product_id)::bigint)
  + (SELECT COALESCE(SUM(h.quantity_on_hand), 0) FROM fixed_barcode_holdings h JOIN units u ON u.id = h.unit_id
     WHERE h.owner_location_id = sqlc.arg(location_id)::bigint AND u.product_id = sqlc.arg(product_id)::bigint)
)::bigint AS pieces;

-- name: MigratorUnitHasLedgerMovements :one
-- TEC-258: a movement this application wrote (not an imported legacy one).
SELECT EXISTS (
    SELECT 1 FROM stock_movements
    WHERE unit_id = sqlc.arg(unit_id)::bigint AND idempotency_key NOT LIKE 'legacy:%'
);

-- name: MigratorUnitsWithoutMovements :many
-- TEC-258: units of the brand whose ownership was written without a movement
-- (TEC-257): they need an opening movement before the projection rebuild.
SELECT u.id FROM units u
WHERE u.brand_id = sqlc.arg(brand_id)
  AND (EXISTS (SELECT 1 FROM unit_current_state s WHERE s.unit_id = u.id)
       OR EXISTS (SELECT 1 FROM fixed_barcode_holdings h WHERE h.unit_id = u.id))
  AND NOT EXISTS (SELECT 1 FROM stock_movements m WHERE m.unit_id = u.id)
ORDER BY u.id;
