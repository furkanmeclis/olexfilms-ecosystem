DROP INDEX IF EXISTS idx_organizations_located;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_longitude_range,
    DROP CONSTRAINT IF EXISTS chk_organizations_latitude_range,
    DROP CONSTRAINT IF EXISTS chk_organizations_coordinates_pair,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude;
