DELETE FROM connection_location_maps WHERE warehouse_location_id IS NULL;
DROP INDEX IF EXISTS idx_connection_location_maps_warehouse;
ALTER TABLE connection_location_maps
    DROP CONSTRAINT IF EXISTS fk_connection_location_maps_warehouse,
    DROP CONSTRAINT IF EXISTS uq_connection_location_maps_uuid,
    ALTER COLUMN warehouse_location_id SET NOT NULL,
    DROP COLUMN IF EXISTS warehouse_id,
    DROP COLUMN IF EXISTS uuid;
