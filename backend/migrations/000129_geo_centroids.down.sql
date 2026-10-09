ALTER TABLE districts
    DROP CONSTRAINT IF EXISTS chk_districts_longitude_range,
    DROP CONSTRAINT IF EXISTS chk_districts_latitude_range,
    DROP CONSTRAINT IF EXISTS chk_districts_centroid_pair,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude;

ALTER TABLE provinces
    DROP CONSTRAINT IF EXISTS chk_provinces_longitude_range,
    DROP CONSTRAINT IF EXISTS chk_provinces_latitude_range,
    DROP CONSTRAINT IF EXISTS chk_provinces_centroid_pair,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude;
