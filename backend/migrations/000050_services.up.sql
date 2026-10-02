-- TEC-178 (F1-05a): service schema, stock owner FK, service permissions.
-- Decisions: TEC-97 orchestrator comment items 1-7, TEC-154 note (service
-- owner_id is BIGINT, FK added here).
--
--   * services: one vehicle application by a dealer (or distributor /
--     center). organization_id is the organization performing the service,
--     brand_id that organization's brand (decision 2). The vehicle is kept
--     as vehicle_id and also as a snapshot (plate, plate_country, VIN, car
--     brand/model, year; decision 3) so PDFs and warranties stay stable when
--     the vehicle changes later. package is free text in F1 (decision 4).
--     The measurement answer (has_measurement) needs a VIN; the report and
--     the contract are plain ids until TEC-113/TEC-112 add their FKs.
--   * service_items: stock used by the service. full = the whole unit
--     (serial piece, whole roll, n pieces of a fixed barcode), partial = a
--     cut of meters from a roll. Parts must come from the product
--     category's available_parts. Items are locked once the service is
--     completed or cancelled (decision 1: not even the center edits them).
--   * service_images, service_status_logs (append-only).
--   * unit_current_state / fixed_barcode_holdings: owner_service_id turns
--     owner_type 'service' into a real FK to services(id, organization_id);
--     the holder organization is the service organization (ledger.Owner
--     OrgID for service owners).
--
-- Status list (legacy ServiceStatusEnum): draft, pending, processing,
-- ready, completed, cancelled. completed and cancelled are final in F1
-- (no completed -> cancelled, so no return movement is needed).
--
-- Stock consumption happens only on the completed transition, in one
-- transaction (decision 6); the use case writes service_items
-- .stock_movement_id before it flips the status, because the items are
-- locked afterwards.

-- 1. Services.
CREATE TABLE services (
    id                     BIGSERIAL PRIMARY KEY,
    uuid                   UUID          NOT NULL DEFAULT gen_random_uuid(),
    service_no             VARCHAR(32)   NOT NULL,
    organization_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id               BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    customer_user_id       BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    vehicle_id             BIGINT        NOT NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    -- Vehicle snapshot (decision 3).
    car_brand_id           BIGINT        NOT NULL REFERENCES car_brands (id) ON DELETE RESTRICT,
    car_model_id           BIGINT        NOT NULL REFERENCES car_models (id) ON DELETE RESTRICT,
    model_year             SMALLINT      NULL,
    plate                  VARCHAR(20)   NULL,
    plate_country          CHAR(2)       NULL,
    vin                    VARCHAR(17)   NULL,
    km                     INT           NULL,
    -- Content.
    package                VARCHAR(255)  NULL,
    notes                  TEXT          NULL,
    -- Measurement answer and placeholders (TEC-113, TEC-112).
    has_measurement        BOOLEAN       NOT NULL DEFAULT false,
    measurement_result_id  BIGINT        NULL,
    contract_id            BIGINT        NULL,
    -- Status and actors.
    status                 VARCHAR(16)   NOT NULL DEFAULT 'draft',
    created_by_user_id     BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    updated_by_user_id     BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    completed_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    cancelled_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    cancel_reason          TEXT          NULL,
    completed_at           TIMESTAMPTZ   NULL,
    cancelled_at           TIMESTAMPTZ   NULL,
    review_request_sent_at TIMESTAMPTZ   NULL,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_services_uuid UNIQUE (uuid),
    -- Public warranty / PDF lookup (DS + 8 digits from the use case).
    CONSTRAINT uq_services_service_no UNIQUE (service_no),
    -- Target of the stock owner FKs (holder = service organization).
    CONSTRAINT uq_services_id_org UNIQUE (id, organization_id),
    -- Target of the organization/brand-consistent FKs of the child tables.
    CONSTRAINT uq_services_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_services_service_no CHECK (btrim(service_no) <> ''),
    CONSTRAINT chk_services_model_year CHECK (model_year IS NULL OR model_year BETWEEN 1900 AND 2100),
    CONSTRAINT chk_services_plate CHECK (plate IS NULL OR (btrim(plate) <> '' AND plate_country IS NOT NULL)),
    CONSTRAINT chk_services_plate_country CHECK (plate_country IS NULL OR plate_country ~ '^[A-Z]{2}$'),
    -- 17 characters without I, O and Q (same rule as vehicles).
    CONSTRAINT chk_services_vin CHECK (vin IS NULL OR vin ~ '^[A-HJ-NPR-Z0-9]{17}$'),
    -- A measurement is matched by VIN, so the answer "yes" needs one.
    CONSTRAINT chk_services_measurement_vin CHECK (NOT has_measurement OR vin IS NOT NULL),
    CONSTRAINT chk_services_km CHECK (km IS NULL OR km >= 0),
    CONSTRAINT chk_services_status CHECK (status IN (
        'draft', 'pending', 'processing', 'ready', 'completed', 'cancelled'
    )),
    CONSTRAINT chk_services_completed CHECK ((status = 'completed') = (completed_at IS NOT NULL)),
    CONSTRAINT chk_services_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

CREATE INDEX idx_services_org_created ON services (organization_id, created_at);
CREATE INDEX idx_services_org_status ON services (organization_id, status, created_at);
CREATE INDEX idx_services_brand_created ON services (brand_id, created_at);
-- TEC-151 (top-10 chart): completed services per car brand/model.
CREATE INDEX idx_services_brand_car ON services (brand_id, car_brand_id, car_model_id, completed_at);
CREATE INDEX idx_services_customer ON services (customer_user_id, created_at);
CREATE INDEX idx_services_vehicle ON services (vehicle_id);
CREATE INDEX idx_services_vin ON services (vin) WHERE vin IS NOT NULL;
CREATE INDEX idx_services_created_by ON services (created_by_user_id) WHERE created_by_user_id IS NOT NULL;

CREATE TRIGGER trg_services_set_updated_at
    BEFORE UPDATE ON services
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The brand is the organization's brand (decision 2), the car model belongs
-- to the car brand, and completed / cancelled are final (F1 has no
-- completed -> cancelled; the stock consumed by a completed service stays
-- consumed).
CREATE FUNCTION services_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand   BIGINT;
    model_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status IN ('completed', 'cancelled')
       AND NEW.status IS DISTINCT FROM OLD.status THEN
        RAISE EXCEPTION 'services: status % is final (service %)', OLD.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'INSERT' OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
       OR NEW.brand_id IS DISTINCT FROM OLD.brand_id THEN
        SELECT brand_id INTO org_brand FROM organizations WHERE id = NEW.organization_id;
        IF FOUND AND org_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'services: brand % does not match organization % (brand %)',
                NEW.brand_id, NEW.organization_id, org_brand
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    -- The vehicle belongs to the customer when the pair is written; a later
    -- ownership transfer (F1-06) does not rewrite past services.
    IF TG_OP = 'INSERT' OR NEW.vehicle_id IS DISTINCT FROM OLD.vehicle_id
       OR NEW.customer_user_id IS DISTINCT FROM OLD.customer_user_id THEN
        IF EXISTS (SELECT 1 FROM vehicles v
                   WHERE v.id = NEW.vehicle_id AND v.user_id IS DISTINCT FROM NEW.customer_user_id) THEN
            RAISE EXCEPTION 'services: vehicle % does not belong to user %',
                NEW.vehicle_id, NEW.customer_user_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    IF TG_OP = 'INSERT' OR NEW.car_model_id IS DISTINCT FROM OLD.car_model_id
       OR NEW.car_brand_id IS DISTINCT FROM OLD.car_brand_id THEN
        SELECT car_brand_id INTO model_brand FROM car_models WHERE id = NEW.car_model_id;
        IF FOUND AND model_brand IS DISTINCT FROM NEW.car_brand_id THEN
            RAISE EXCEPTION 'services: car model % belongs to car brand %, not %',
                NEW.car_model_id, model_brand, NEW.car_brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_services_check_row
    BEFORE INSERT OR UPDATE ON services
    FOR EACH ROW
    EXECUTE FUNCTION services_check_row();

-- 2. Service items. One row per unit used; the same roll may appear twice
-- (two cuts), so (service_id, unit_id) is not unique. kind full: the whole
-- serial unit (quantity NULL) or n pieces of a fixed barcode (quantity);
-- kind partial: meters cut from a roll. applied_parts is a JSON array of
-- part keys taken from the category's available_parts.
CREATE TABLE service_items (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID           NOT NULL DEFAULT gen_random_uuid(),
    service_id         BIGINT         NOT NULL,
    organization_id    BIGINT         NOT NULL,
    brand_id           BIGINT         NOT NULL,
    product_id         BIGINT         NOT NULL,
    unit_id            BIGINT         NOT NULL,
    kind               VARCHAR(16)    NOT NULL,
    quantity           INT            NULL,
    meters             NUMERIC(10,2)  NULL,
    applied_parts      JSONB          NOT NULL DEFAULT '[]'::jsonb,
    notes              TEXT           NULL,
    -- Ledger movement written on completion (idempotency key
    -- service:service_item:<id>, decision 6).
    stock_movement_id  BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_items_uuid UNIQUE (uuid),
    CONSTRAINT fk_service_items_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_service_items_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    -- A Glorian unit never enters an Olex service (K1/K20).
    CONSTRAINT fk_service_items_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_items_kind CHECK (kind IN ('full', 'partial')),
    CONSTRAINT chk_service_items_amount CHECK (
        (kind = 'partial') = (meters IS NOT NULL)
        AND (kind = 'full' OR quantity IS NULL)
        AND (quantity IS NULL OR quantity > 0)
        AND (meters IS NULL OR meters > 0)
    ),
    CONSTRAINT chk_service_items_parts CHECK (jsonb_typeof(applied_parts) = 'array')
);

CREATE INDEX idx_service_items_service ON service_items (service_id, id);
CREATE INDEX idx_service_items_unit ON service_items (unit_id);

CREATE TRIGGER trg_service_items_set_updated_at
    BEFORE UPDATE ON service_items
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Lock (decision 1): no insert, update or delete once the service is
-- completed or cancelled. The amount must fit the unit (partial only from a
-- roll, quantity only for a fixed barcode), the unit must be of the
-- product, and the parts must be in the category's available_parts.
CREATE FUNCTION service_items_check() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    svc_status  VARCHAR(16);
    u_product   BIGINT;
    u_kind      VARCHAR(16);
    p_unit_type VARCHAR(16);
    p_fixed     BOOLEAN;
    c_parts     JSONB;
    sid         BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        sid := OLD.service_id;
    ELSE
        sid := NEW.service_id;
    END IF;
    SELECT status INTO svc_status FROM services WHERE id = sid;
    IF svc_status IN ('completed', 'cancelled') THEN
        RAISE EXCEPTION 'service_items: service % is %; its items are locked', sid, svc_status
            USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.service_id IS DISTINCT FROM NEW.service_id THEN
        SELECT status INTO svc_status FROM services WHERE id = OLD.service_id;
        IF svc_status IN ('completed', 'cancelled') THEN
            RAISE EXCEPTION 'service_items: service % is %; its items are locked', OLD.service_id, svc_status
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    SELECT u.product_id, u.unit_kind INTO u_product, u_kind FROM units u WHERE u.id = NEW.unit_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF u_product IS DISTINCT FROM NEW.product_id THEN
        RAISE EXCEPTION 'service_items: unit % is of product %, not %', NEW.unit_id, u_product, NEW.product_id
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT p.unit_type, p.uses_fixed_barcode, c.available_parts
      INTO p_unit_type, p_fixed, c_parts
      FROM products p JOIN product_categories c ON c.id = p.category_id
     WHERE p.id = NEW.product_id;
    IF NEW.kind = 'partial' AND (u_kind <> 'serial' OR p_unit_type <> 'roll_meter') THEN
        RAISE EXCEPTION 'service_items: a partial item needs a roll (unit %)', NEW.unit_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.kind = 'full' AND (u_kind = 'fixed') <> (NEW.quantity IS NOT NULL) THEN
        RAISE EXCEPTION 'service_items: quantity is set exactly for fixed barcodes (unit %)', NEW.unit_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NOT (NEW.applied_parts <@ COALESCE(c_parts, '[]'::jsonb))
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(NEW.applied_parts) e WHERE jsonb_typeof(e) <> 'string') THEN
        RAISE EXCEPTION 'service_items: parts % are not in the available parts of product %',
            NEW.applied_parts, NEW.product_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_items_check
    BEFORE INSERT OR UPDATE OR DELETE ON service_items
    FOR EACH ROW
    EXECUTE FUNCTION service_items_check();

-- 3. Images (storage keys, display order).
CREATE TABLE service_images (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    service_id       BIGINT        NOT NULL,
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    storage_key      TEXT          NOT NULL,
    title            VARCHAR(255)  NULL,
    sort_order       INT           NOT NULL DEFAULT 0,
    uploaded_by_user_id BIGINT     NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_images_uuid UNIQUE (uuid),
    CONSTRAINT fk_service_images_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_images_storage_key CHECK (btrim(storage_key) <> '')
);

CREATE INDEX idx_service_images_service ON service_images (service_id, sort_order, id);

-- 4. Status log (append-only). actor_org_id tells "a note of another
-- organization" (e.g. the center) apart from the service organization.
CREATE TABLE service_status_logs (
    id               BIGSERIAL PRIMARY KEY,
    service_id       BIGINT        NOT NULL,
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    from_status      VARCHAR(16)   NULL,
    to_status        VARCHAR(16)   NOT NULL,
    actor_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    actor_org_id     BIGINT        NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    note             TEXT          NULL,
    metadata         JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_service_status_logs_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_status_logs_from CHECK (from_status IS NULL OR from_status IN (
        'draft', 'pending', 'processing', 'ready', 'completed', 'cancelled'
    )),
    CONSTRAINT chk_service_status_logs_to CHECK (to_status IN (
        'draft', 'pending', 'processing', 'ready', 'completed', 'cancelled'
    )),
    CONSTRAINT chk_service_status_logs_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_service_status_logs_service ON service_status_logs (service_id, created_at, id);
CREATE INDEX idx_service_status_logs_org_created ON service_status_logs (organization_id, created_at);

CREATE FUNCTION service_status_logs_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'service_status_logs is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_service_status_logs_append_only
    BEFORE UPDATE OR DELETE ON service_status_logs
    FOR EACH ROW
    EXECUTE FUNCTION service_status_logs_append_only();

CREATE TRIGGER trg_service_status_logs_no_truncate
    BEFORE TRUNCATE ON service_status_logs
    FOR EACH STATEMENT
    EXECUTE FUNCTION service_status_logs_append_only();

-- 5. Stock owner FK (TEC-154 note). Like the location and organization
-- owners, a stored generated column turns owner_type 'service' into a real
-- FK; the holder organization must be the service organization. The CHECK
-- states the invariant explicitly.
ALTER TABLE unit_current_state
    ADD COLUMN owner_service_id BIGINT GENERATED ALWAYS AS (
        CASE WHEN owner_type = 'service' THEN owner_id END
    ) STORED,
    ADD CONSTRAINT fk_unit_current_state_owner_service FOREIGN KEY (owner_service_id, holder_org_id)
        REFERENCES services (id, organization_id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_unit_current_state_owner_service CHECK (
        owner_type <> 'service' OR (owner_service_id IS NOT NULL AND owner_service_id = owner_id)
    );

CREATE INDEX idx_unit_current_state_owner_service ON unit_current_state (owner_service_id)
    WHERE owner_service_id IS NOT NULL;

ALTER TABLE fixed_barcode_holdings
    ADD COLUMN owner_service_id BIGINT GENERATED ALWAYS AS (
        CASE WHEN owner_type = 'service' THEN owner_id END
    ) STORED,
    ADD CONSTRAINT fk_fixed_barcode_holdings_owner_service FOREIGN KEY (owner_service_id, holder_org_id)
        REFERENCES services (id, organization_id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_fixed_barcode_holdings_owner_service CHECK (
        owner_type <> 'service' OR (owner_service_id IS NOT NULL AND owner_service_id = owner_id)
    );

CREATE INDEX idx_fixed_barcode_holdings_owner_service ON fixed_barcode_holdings (owner_service_id)
    WHERE owner_service_id IS NOT NULL;

-- 6. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (TestMigrationMatchesCatalog compares the two). services.read/write exist
-- since 000029; complete and cancel are new. cancel is center-only
-- (scopes brand/all).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Complete services', 'services.complete', 'services',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Complete a service: consumes its stock and starts the warranty flow.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Cancel services', 'services.cancel', 'services',
       ARRAY['brand', 'all']::text[], false, false,
       'Cancel a service that is not completed (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'services.complete', 'all'),
    ('super_admin', 'services.cancel', 'all'),
    ('center_staff', 'services.complete', 'brand'),
    ('center_staff', 'services.cancel', 'brand'),
    ('distributor_owner', 'services.complete', 'subtree'),
    ('distributor_staff', 'services.complete', 'subtree'),
    ('dealer_owner', 'services.complete', 'managed'),
    ('dealer_staff', 'services.complete', 'own')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
