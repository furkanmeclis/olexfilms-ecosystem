-- TEC-201: warehouse and location tree (000059). Every query is bound to
-- one organization; the warehouse side is brand-independent (K20).

-- ---------------------------------------------------------------------------
-- Warehouses.

-- name: ListWarehouses :many
SELECT * FROM warehouses
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
ORDER BY sort_order, id;

-- name: GetWarehouseByUUID :one
SELECT * FROM warehouses
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: GetWarehouseByID :one
SELECT * FROM warehouses
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: ListWarehousesByUUIDs :many
SELECT * FROM warehouses
WHERE uuid = ANY(sqlc.arg(uuids)::uuid[]) AND organization_id = sqlc.arg(organization_id);

-- name: CreateWarehouse :one
INSERT INTO warehouses (organization_id, code, name, address, active, sort_order)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.narg(address),
    sqlc.arg(active),
    (SELECT COALESCE(MAX(w.sort_order) + 1, 0) FROM warehouses w
     WHERE w.organization_id = sqlc.arg(organization_id))
)
RETURNING *;

-- name: UpdateWarehouse :one
UPDATE warehouses
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    address = sqlc.narg(address),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: DeleteWarehouse :execrows
DELETE FROM warehouses
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: SetWarehouseSortOrder :execrows
UPDATE warehouses SET sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- ---------------------------------------------------------------------------
-- Rooms.

-- name: ListRooms :many
SELECT * FROM rooms
WHERE warehouse_id = sqlc.arg(warehouse_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY sort_order, id;

-- name: GetRoomByUUID :one
SELECT * FROM rooms
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: GetRoomByID :one
SELECT * FROM rooms
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: ListRoomsByUUIDs :many
SELECT * FROM rooms
WHERE uuid = ANY(sqlc.arg(uuids)::uuid[]) AND organization_id = sqlc.arg(organization_id);

-- name: CreateRoom :one
INSERT INTO rooms (organization_id, warehouse_id, code, name, active, sort_order)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(warehouse_id), sqlc.arg(code), sqlc.arg(name),
    sqlc.arg(active),
    (SELECT COALESCE(MAX(r.sort_order) + 1, 0) FROM rooms r
     WHERE r.warehouse_id = sqlc.arg(warehouse_id))
)
RETURNING *;

-- name: UpdateRoom :one
UPDATE rooms
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: DeleteRoom :execrows
DELETE FROM rooms
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: SetRoomSortOrder :execrows
UPDATE rooms SET sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- ---------------------------------------------------------------------------
-- Typed locations (aisle, shelf, bin).

-- name: ListRoomLocations :many
-- The whole tree of a room, parents before children is not guaranteed:
-- callers group by parent_id and order siblings by sort_order.
SELECT * FROM warehouse_locations
WHERE room_id = sqlc.arg(room_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY parent_id NULLS FIRST, sort_order, id;

-- name: GetTypedLocationByUUID :one
SELECT * FROM warehouse_locations
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
  AND room_id IS NOT NULL;

-- name: ListLocationsByUUIDs :many
SELECT * FROM warehouse_locations
WHERE uuid = ANY(sqlc.arg(uuids)::uuid[]) AND organization_id = sqlc.arg(organization_id);

-- name: CreateTypedLocation :one
INSERT INTO warehouse_locations (
    organization_id, room_id, parent_id, type, code, name, active, sort_order
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(room_id), sqlc.narg(parent_id)::bigint, sqlc.arg(type),
    sqlc.arg(code), sqlc.arg(name), sqlc.arg(active),
    (SELECT COALESCE(MAX(l.sort_order) + 1, 0) FROM warehouse_locations l
     WHERE l.room_id = sqlc.arg(room_id)
       AND l.parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::bigint)
)
RETURNING *;

-- name: FindTypedLocationChild :one
-- The sibling with this code (bulk generation reuses existing nodes).
SELECT * FROM warehouse_locations
WHERE room_id = sqlc.arg(room_id)
  AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::bigint
  AND code = sqlc.arg(code);

-- name: UpdateTypedLocation :one
UPDATE warehouse_locations
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND room_id IS NOT NULL
RETURNING *;

-- name: DeleteTypedLocation :execrows
DELETE FROM warehouse_locations
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND room_id IS NOT NULL;

-- name: SetLocationSortOrder :execrows
UPDATE warehouse_locations SET sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);
