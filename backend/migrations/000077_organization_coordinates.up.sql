-- TEC-240: dealer coordinates for the "find a dealer" map and the public
-- GET /v1/public/dealers/nearby endpoint. Both columns are set together or
-- both are NULL; ranges are WGS84 degrees.
ALTER TABLE organizations
    ADD COLUMN latitude  NUMERIC(9,6) NULL,
    ADD COLUMN longitude NUMERIC(9,6) NULL,
    ADD CONSTRAINT chk_organizations_coordinates_pair
        CHECK ((latitude IS NULL) = (longitude IS NULL)),
    ADD CONSTRAINT chk_organizations_latitude_range
        CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    ADD CONSTRAINT chk_organizations_longitude_range
        CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180);

CREATE INDEX idx_organizations_located
    ON organizations (brand_id, type)
    WHERE latitude IS NOT NULL AND deleted_at IS NULL;
