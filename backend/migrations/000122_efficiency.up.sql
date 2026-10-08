-- TEC-487 (F5-06a): efficiency and waste analytics schema.
-- The projections are fed from partial service_items (roll-meter cuts) and
-- can be rebuilt by service item when a consumption correction is recorded.

CREATE TABLE part_consumption_expectations (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id       BIGINT        NULL,
    category_id      BIGINT        NULL,
    body_type        VARCHAR(64)   NULL,
    part_key         VARCHAR(100)  NOT NULL,
    expected_meters  NUMERIC(10,2) NOT NULL,
    source           VARCHAR(16)   NOT NULL DEFAULT 'manual',
    sample_size      INTEGER       NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_part_consumption_expectations_uuid UNIQUE (uuid),
    CONSTRAINT fk_part_consumption_expectations_product
        FOREIGN KEY (product_id, brand_id) REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_part_consumption_expectations_category
        FOREIGN KEY (category_id, brand_id) REFERENCES product_categories (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_part_consumption_expectations_target CHECK (
        (product_id IS NULL) <> (category_id IS NULL)
    ),
    CONSTRAINT chk_part_consumption_expectations_part_key CHECK (btrim(part_key) <> ''),
    CONSTRAINT chk_part_consumption_expectations_expected CHECK (expected_meters > 0),
    CONSTRAINT chk_part_consumption_expectations_source CHECK (source IN ('manual', 'network')),
    CONSTRAINT chk_part_consumption_expectations_sample CHECK (sample_size >= 0),
    CONSTRAINT chk_part_consumption_expectations_body_type CHECK (body_type IS NULL OR btrim(body_type) <> '')
);

CREATE UNIQUE INDEX uq_part_expect_product_body_part
    ON part_consumption_expectations (brand_id, product_id, body_type, part_key)
    WHERE product_id IS NOT NULL AND body_type IS NOT NULL;
CREATE UNIQUE INDEX uq_part_expect_product_all_part
    ON part_consumption_expectations (brand_id, product_id, part_key)
    WHERE product_id IS NOT NULL AND body_type IS NULL;
CREATE UNIQUE INDEX uq_part_expect_category_body_part
    ON part_consumption_expectations (brand_id, category_id, body_type, part_key)
    WHERE category_id IS NOT NULL AND body_type IS NOT NULL;
CREATE UNIQUE INDEX uq_part_expect_category_all_part
    ON part_consumption_expectations (brand_id, category_id, part_key)
    WHERE category_id IS NOT NULL AND body_type IS NULL;
CREATE INDEX idx_part_expectations_brand_updated
    ON part_consumption_expectations (brand_id, updated_at DESC, id);

CREATE TRIGGER trg_part_consumption_expectations_set_updated_at
    BEFORE UPDATE ON part_consumption_expectations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION part_consumption_expectations_check() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type    TEXT;
    org_brand   BIGINT;
    parts       JSONB;
BEGIN
    SELECT type, brand_id INTO org_type, org_brand
    FROM organizations
    WHERE id = NEW.organization_id;
    IF FOUND AND (org_type IS DISTINCT FROM 'center' OR org_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'part_consumption_expectations: owner % must be the center organization of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;

    IF NEW.product_id IS NOT NULL THEN
        SELECT c.available_parts INTO parts
        FROM products p
        JOIN product_categories c ON c.id = p.category_id
        WHERE p.id = NEW.product_id AND p.brand_id = NEW.brand_id;
    ELSE
        SELECT c.available_parts INTO parts
        FROM product_categories c
        WHERE c.id = NEW.category_id AND c.brand_id = NEW.brand_id;
    END IF;

    IF parts IS NOT NULL AND NOT (parts ? NEW.part_key) THEN
        RAISE EXCEPTION 'part_consumption_expectations: part % is not available for target',
            NEW.part_key
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_part_consumption_expectations_check
    BEFORE INSERT OR UPDATE OF organization_id, brand_id, product_id, category_id, part_key
    ON part_consumption_expectations
    FOR EACH ROW
    EXECUTE FUNCTION part_consumption_expectations_check();

CREATE TABLE efficiency_facts (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    service_id       BIGINT        NOT NULL,
    service_item_id  BIGINT        NOT NULL,
    unit_id          BIGINT        NOT NULL,
    product_id       BIGINT        NOT NULL,
    dealer_org_id    BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    staff_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    body_type        VARCHAR(64)   NULL,
    part_key         VARCHAR(100)  NOT NULL,
    actual_meters    NUMERIC(10,2) NOT NULL,
    expected_meters  NUMERIC(10,2) NULL,
    waste_ratio      NUMERIC(12,6) GENERATED ALWAYS AS (
        CASE WHEN expected_meters IS NULL THEN NULL
             ELSE (actual_meters / expected_meters) - 1
        END
    ) STORED,
    service_date     DATE          NOT NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_efficiency_facts_uuid UNIQUE (uuid),
    CONSTRAINT uq_efficiency_facts_item_part UNIQUE (service_item_id, part_key),
    CONSTRAINT fk_efficiency_facts_item
        FOREIGN KEY (service_item_id) REFERENCES service_items (id) ON DELETE CASCADE,
    CONSTRAINT fk_efficiency_facts_service
        FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_efficiency_facts_product
        FOREIGN KEY (product_id, brand_id) REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_efficiency_facts_unit
        FOREIGN KEY (unit_id, brand_id) REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_efficiency_facts_part CHECK (btrim(part_key) <> ''),
    CONSTRAINT chk_efficiency_facts_actual CHECK (actual_meters >= 0),
    CONSTRAINT chk_efficiency_facts_expected CHECK (expected_meters IS NULL OR expected_meters > 0)
);

CREATE INDEX idx_efficiency_facts_brand_date ON efficiency_facts (brand_id, service_date, id);
CREATE INDEX idx_efficiency_facts_dealer_date ON efficiency_facts (dealer_org_id, service_date, id);
CREATE INDEX idx_efficiency_facts_staff_date ON efficiency_facts (staff_user_id, service_date, id)
    WHERE staff_user_id IS NOT NULL;
CREATE INDEX idx_efficiency_facts_product_date ON efficiency_facts (product_id, service_date, id);
CREATE INDEX idx_efficiency_facts_part_date ON efficiency_facts (part_key, service_date, id);
CREATE INDEX idx_efficiency_facts_unit ON efficiency_facts (unit_id);

CREATE TRIGGER trg_efficiency_facts_set_updated_at
    BEFORE UPDATE ON efficiency_facts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE roll_efficiency (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    unit_id          BIGINT        NOT NULL,
    product_id       BIGINT        NOT NULL,
    initial_meters   NUMERIC(10,2) NOT NULL,
    consumed_meters  NUMERIC(10,2) NOT NULL DEFAULT 0,
    expected_meters  NUMERIC(10,2) NOT NULL DEFAULT 0,
    waste_meters     NUMERIC(10,2) GENERATED ALWAYS AS (consumed_meters - expected_meters) STORED,
    remaining_meters NUMERIC(10,2) NOT NULL DEFAULT 0,
    service_count    INTEGER       NOT NULL DEFAULT 0,
    last_used_at     TIMESTAMPTZ   NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_roll_efficiency_uuid UNIQUE (uuid),
    CONSTRAINT uq_roll_efficiency_unit UNIQUE (unit_id),
    CONSTRAINT fk_roll_efficiency_unit
        FOREIGN KEY (unit_id, brand_id) REFERENCES units (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_roll_efficiency_product
        FOREIGN KEY (product_id, brand_id) REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_roll_efficiency_meters CHECK (
        initial_meters >= 0 AND consumed_meters >= 0 AND expected_meters >= 0 AND remaining_meters >= 0
    ),
    CONSTRAINT chk_roll_efficiency_service_count CHECK (service_count >= 0)
);

CREATE INDEX idx_roll_efficiency_brand_waste ON roll_efficiency (brand_id, waste_meters DESC, id);
CREATE INDEX idx_roll_efficiency_product ON roll_efficiency (product_id, id);
CREATE INDEX idx_roll_efficiency_last_used ON roll_efficiency (last_used_at DESC NULLS LAST, id);

CREATE TRIGGER trg_roll_efficiency_set_updated_at
    BEFORE UPDATE ON roll_efficiency
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Permissions. Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read efficiency analytics', 'efficiency.read', 'efficiency',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read part consumption, roll efficiency and waste analytics in scope.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage efficiency expectations', 'efficiency.expectations.manage', 'efficiency',
       ARRAY['brand', 'all']::text[], false, false,
       'Create and update expected part consumption definitions.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'efficiency.read', 'all'),
    ('super_admin', 'efficiency.expectations.manage', 'all'),
    ('center_staff', 'efficiency.read', 'brand'),
    ('center_staff', 'efficiency.expectations.manage', 'brand'),
    ('distributor_owner', 'efficiency.read', 'subtree'),
    ('dealer_owner', 'efficiency.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
