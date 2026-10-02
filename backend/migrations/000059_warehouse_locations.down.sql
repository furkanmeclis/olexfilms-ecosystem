-- Reverts TEC-201. Typed locations (bins first, the parent foreign key is
-- RESTRICT) are removed; legacy locations stay. A typed location still
-- referenced by the ledger blocks the rollback on purpose.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r
JOIN permissions p ON p.slug = 'warehouse.read'
WHERE r.slug = 'dealer_owner'
ON CONFLICT DO NOTHING;

DROP TRIGGER IF EXISTS trg_warehouses_cascade_code ON warehouses;
DROP TRIGGER IF EXISTS trg_rooms_cascade_code ON rooms;
DROP TRIGGER IF EXISTS trg_warehouse_locations_cascade ON warehouse_locations;
DROP TRIGGER IF EXISTS trg_warehouse_locations_derive ON warehouse_locations;
DROP FUNCTION IF EXISTS warehouses_cascade_code();
DROP FUNCTION IF EXISTS rooms_cascade_code();
DROP FUNCTION IF EXISTS warehouse_locations_cascade();
DROP FUNCTION IF EXISTS warehouse_locations_derive();

DELETE FROM warehouse_locations WHERE type = 'bin';
DELETE FROM warehouse_locations WHERE type = 'shelf';
DELETE FROM warehouse_locations WHERE type = 'aisle';

DROP INDEX IF EXISTS idx_warehouse_locations_room_sort;
DROP INDEX IF EXISTS uq_warehouse_locations_org_full_code;
DROP INDEX IF EXISTS uq_warehouse_locations_org_code_legacy;
ALTER TABLE warehouse_locations
    ADD CONSTRAINT uq_warehouse_locations_org_code UNIQUE (organization_id, code);

ALTER TABLE warehouse_locations
    DROP CONSTRAINT IF EXISTS chk_warehouse_locations_typed_code,
    DROP CONSTRAINT IF EXISTS chk_warehouse_locations_tree,
    DROP CONSTRAINT IF EXISTS chk_warehouse_locations_type,
    DROP CONSTRAINT IF EXISTS fk_warehouse_locations_room,
    DROP CONSTRAINT IF EXISTS fk_warehouse_locations_warehouse;

ALTER TABLE warehouse_locations
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS full_code,
    DROP COLUMN IF EXISTS type,
    DROP COLUMN IF EXISTS room_id,
    DROP COLUMN IF EXISTS warehouse_id;

DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS warehouses;
