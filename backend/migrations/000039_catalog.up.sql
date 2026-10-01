-- TEC-144 (F1-01a): product catalog. The catalog is brand master data written
-- only by the center (K4) and read by distributors and dealers of the same
-- brand (K1/K20). Prices live in 000040.

-- 1. Brand currency: product prices default to the brand currency (K7/K8).
ALTER TABLE brands
    ADD COLUMN currency CHAR(3) NOT NULL DEFAULT 'TRY',
    ADD CONSTRAINT chk_brands_currency CHECK (currency ~ '^[A-Z]{3}$');

-- Catalog rows belong to the center organization of their brand.
CREATE FUNCTION catalog_check_center_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type  TEXT;
    org_brand BIGINT;
BEGIN
    SELECT type, brand_id INTO org_type, org_brand
    FROM organizations WHERE id = NEW.organization_id;
    IF org_type IS DISTINCT FROM 'center' OR org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'catalog owner % must be the center organization of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

-- 2. Categories. available_parts lists the vehicle parts a service may pick
-- for products of the category (legacy ProductCategory.available_parts).
CREATE TABLE product_categories (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name             VARCHAR(200) NOT NULL,
    available_parts  JSONB        NOT NULL DEFAULT '[]'::jsonb,
    sort             INT          NOT NULL DEFAULT 0,
    active           BOOLEAN      NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_categories_uuid UNIQUE (uuid),
    CONSTRAINT uq_product_categories_brand_name UNIQUE (brand_id, name),
    -- Target of the brand-consistent FK from products.
    CONSTRAINT uq_product_categories_id_brand UNIQUE (id, brand_id),
    CONSTRAINT chk_product_categories_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_product_categories_parts CHECK (jsonb_typeof(available_parts) = 'array')
);

CREATE INDEX idx_product_categories_brand_sort ON product_categories (brand_id, sort, id);

CREATE TRIGGER trg_product_categories_set_updated_at
    BEFORE UPDATE ON product_categories
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_product_categories_center_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON product_categories
    FOR EACH ROW
    EXECUTE FUNCTION catalog_check_center_org();

-- 3. Products. external_id / connection_id / locked_fields are reserved for
-- the F2 Glorian catalog sync (K2); nothing writes them yet.
CREATE TABLE products (
    id                        BIGSERIAL PRIMARY KEY,
    uuid                      UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id           BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                  BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    category_id               BIGINT        NOT NULL,
    sku                       VARCHAR(64)   NOT NULL,
    name                      VARCHAR(200)  NOT NULL,
    description_md            TEXT          NOT NULL DEFAULT '',
    warranty_duration_months  INT           NULL,
    micron_thickness          NUMERIC(8,2)  NULL,
    -- Array of storage object descriptors, e.g. [{"key": "...", "sort": 0}].
    images                    JSONB         NOT NULL DEFAULT '[]'::jsonb,
    unit_type                 VARCHAR(16)   NOT NULL DEFAULT 'piece',
    uses_fixed_barcode        BOOLEAN       NOT NULL DEFAULT false,
    active                    BOOLEAN       NOT NULL DEFAULT true,
    external_id               VARCHAR(128)  NULL,
    connection_id             BIGINT        NULL,
    locked_fields             TEXT[]        NULL,
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_products_uuid UNIQUE (uuid),
    CONSTRAINT uq_products_brand_sku UNIQUE (brand_id, sku),
    -- Target of the brand-consistent FKs from the pricing tables (000040).
    CONSTRAINT uq_products_id_brand UNIQUE (id, brand_id),
    -- A product and its category always share the brand (K1).
    CONSTRAINT fk_products_category FOREIGN KEY (category_id, brand_id)
        REFERENCES product_categories (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_products_sku CHECK (btrim(sku) <> ''),
    CONSTRAINT chk_products_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_products_unit_type CHECK (unit_type IN ('piece', 'roll_meter')),
    CONSTRAINT chk_products_warranty CHECK (warranty_duration_months IS NULL OR warranty_duration_months >= 0),
    CONSTRAINT chk_products_micron CHECK (micron_thickness IS NULL OR micron_thickness > 0),
    CONSTRAINT chk_products_images CHECK (jsonb_typeof(images) = 'array')
);

CREATE INDEX idx_products_brand_active ON products (brand_id, active, id);
CREATE INDEX idx_products_category ON products (category_id);
CREATE UNIQUE INDEX uq_products_connection_external
    ON products (connection_id, external_id)
    WHERE connection_id IS NOT NULL AND external_id IS NOT NULL;

CREATE TRIGGER trg_products_set_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_products_center_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON products
    FOR EACH ROW
    EXECUTE FUNCTION catalog_check_center_org();

-- 4. Permissions catalog.read / catalog.write (catalog entries in
-- internal/platform/rbac), appended after the existing catalog. Writes are
-- center-only (K4); every organization role reads its brand's catalog.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read catalog', 'catalog.read', 'catalog', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Product categories and products of the active brand.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write catalog', 'catalog.write', 'catalog', ARRAY['brand', 'all']::text[], false, false,
       'Create and edit product categories and products (center only, K4).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'catalog.read', 'all'),
    ('super_admin', 'catalog.write', 'all'),
    ('center_staff', 'catalog.read', 'brand'),
    ('center_staff', 'catalog.write', 'brand'),
    ('center_warehouse', 'catalog.read', 'brand'),
    ('center_accounting', 'catalog.read', 'brand'),
    ('center_social', 'catalog.read', 'brand'),
    ('distributor_owner', 'catalog.read', 'managed'),
    ('distributor_staff', 'catalog.read', 'managed'),
    ('distributor_warehouse_staff', 'catalog.read', 'managed'),
    ('distributor_accounting', 'catalog.read', 'managed'),
    ('dealer_owner', 'catalog.read', 'managed'),
    ('dealer_staff', 'catalog.read', 'managed'),
    ('dealer_accounting', 'catalog.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
