-- TEC-769 (Depo F4.4): Glorian remote location -> local warehouse / bin.
--
-- 000075 created connection_location_maps keyed by a bin. The legacy
-- ConnectionLocationMap maps a remote location name to a warehouse and an
-- optional bin, and the admin screen addresses rows by uuid, so: add uuid
-- and warehouse_id (backfilled from the bin) and make the bin optional.
-- A bin, when set, must belong to the row's warehouse (use case check).
ALTER TABLE connection_location_maps
    ADD COLUMN uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN warehouse_id BIGINT NULL;

UPDATE connection_location_maps m
SET warehouse_id = l.warehouse_id
FROM warehouse_locations l
WHERE l.id = m.warehouse_location_id;

DELETE FROM connection_location_maps WHERE warehouse_id IS NULL;

ALTER TABLE connection_location_maps
    ALTER COLUMN warehouse_id SET NOT NULL,
    ALTER COLUMN warehouse_location_id DROP NOT NULL,
    ADD CONSTRAINT uq_connection_location_maps_uuid UNIQUE (uuid),
    ADD CONSTRAINT fk_connection_location_maps_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT;

CREATE INDEX idx_connection_location_maps_warehouse ON connection_location_maps (warehouse_id);
