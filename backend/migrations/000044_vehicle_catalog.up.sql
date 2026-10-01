-- TEC-149 (F1-09a): vehicle catalog (car brands and models).
--
-- These are global reference tables, not business tables: the same car
-- brands and models serve every brand (olex, glorian) and every
-- organization, and only super_admin writes them. They therefore carry no
-- organization_id / brand_id (deliberate exception to AGENTS §3); business
-- rows that point at them (services, TEC-97) carry the scope. The model's
-- parent column is car_brand_id so it never reads as the tenant brand_id.
-- external_id keeps the legacy hub id for the F2 import (idempotent upsert).
-- Logos and hero images are storage object keys (raster only, no SVG).

CREATE TABLE car_brands (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    external_id      VARCHAR(64)  NULL,
    name             VARCHAR(150) NOT NULL,
    logo_object_key  TEXT         NULL,
    hero_object_key  TEXT         NULL,
    show_name        BOOLEAN      NOT NULL DEFAULT true,
    logo_height      SMALLINT     NULL,
    active           BOOLEAN      NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_car_brands_uuid UNIQUE (uuid),
    CONSTRAINT uq_car_brands_external_id UNIQUE (external_id),
    CONSTRAINT chk_car_brands_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_car_brands_logo_height CHECK (logo_height IS NULL OR logo_height BETWEEN 8 AND 512)
);

CREATE UNIQUE INDEX uq_car_brands_name ON car_brands (lower(name));
CREATE INDEX idx_car_brands_active_name ON car_brands (active, lower(name));

CREATE TRIGGER trg_car_brands_set_updated_at
    BEFORE UPDATE ON car_brands
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE car_models (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    car_brand_id     BIGINT       NOT NULL REFERENCES car_brands (id) ON DELETE RESTRICT,
    external_id      VARCHAR(64)  NULL,
    name             VARCHAR(200) NOT NULL,
    body_type        VARCHAR(64)  NULL,
    powertrain       VARCHAR(64)  NULL,
    year_start       SMALLINT     NULL,
    year_stop        SMALLINT     NULL,
    hero_object_key  TEXT         NULL,
    active           BOOLEAN      NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_car_models_uuid UNIQUE (uuid),
    CONSTRAINT uq_car_models_external_id UNIQUE (external_id),
    CONSTRAINT chk_car_models_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_car_models_year_start CHECK (year_start IS NULL OR year_start BETWEEN 1900 AND 2100),
    CONSTRAINT chk_car_models_year_stop CHECK (year_stop IS NULL OR year_stop BETWEEN 1900 AND 2100),
    CONSTRAINT chk_car_models_year_range CHECK (year_start IS NULL OR year_stop IS NULL OR year_stop >= year_start)
);

CREATE UNIQUE INDEX uq_car_models_brand_name ON car_models (car_brand_id, lower(name));
CREATE INDEX idx_car_models_active_name ON car_models (active, lower(name));

CREATE TRIGGER trg_car_models_set_updated_at
    BEFORE UPDATE ON car_models
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Permissions. Source of truth: internal/platform/rbac/catalog.go (appended
-- last, so sort_order continues after the current maximum). write is held
-- by super_admin only; read by every organization role and the portal roles.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read vehicle catalog', 'vehicle_catalog.read', 'vehicle_catalog',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all', 'customer']::text[], false, false,
       'Car brands and models (global reference data).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write vehicle catalog', 'vehicle_catalog.write', 'vehicle_catalog', ARRAY['all']::text[], false, false,
       'Create and edit car brands, models, logos and hero images (super_admin only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'vehicle_catalog.read', 'all'),
    ('super_admin', 'vehicle_catalog.write', 'all'),
    ('center_staff', 'vehicle_catalog.read', 'brand'),
    ('center_warehouse', 'vehicle_catalog.read', 'brand'),
    ('center_accounting', 'vehicle_catalog.read', 'brand'),
    ('center_social', 'vehicle_catalog.read', 'brand'),
    ('distributor_owner', 'vehicle_catalog.read', 'managed'),
    ('distributor_staff', 'vehicle_catalog.read', 'managed'),
    ('distributor_warehouse_staff', 'vehicle_catalog.read', 'managed'),
    ('distributor_accounting', 'vehicle_catalog.read', 'managed'),
    ('dealer_owner', 'vehicle_catalog.read', 'managed'),
    ('dealer_staff', 'vehicle_catalog.read', 'managed'),
    ('dealer_accounting', 'vehicle_catalog.read', 'managed'),
    ('customer', 'vehicle_catalog.read', 'customer'),
    ('fleet', 'vehicle_catalog.read', 'customer')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
