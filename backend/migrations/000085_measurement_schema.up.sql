-- TEC-293 (F3-02a): measurement schema extension (K28).
--
-- Extends the minimal TEC-233 storage (000076) instead of rebuilding it:
-- normalized reading rows, tires, device link and the before/after link of a
-- service. Every new column on the existing tables is nullable (or has a
-- default) so the raw uploads of the mobile API and the legacy import of the
-- migrator (TEC-262, source = legacy_import) keep inserting unchanged.
--
-- Source fields: the legacy hub's nexptg_reports, nexptg_report_measurements,
-- nexptg_report_tires and service_nexptg_report tables.

-- 1. measurement_results: device time, device link, customer, body type,
--    normalization mark and the stored PDF.
ALTER TABLE measurement_results
    ADD COLUMN measured_at       TIMESTAMPTZ   NULL,
    ADD COLUMN device_id         BIGINT        NULL,
    ADD COLUMN customer_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN body_type         VARCHAR(64)   NULL,
    ADD COLUMN parsed_at         TIMESTAMPTZ   NULL,
    ADD COLUMN pdf_key           VARCHAR(512)  NULL,
    ADD CONSTRAINT chk_measurement_results_pdf_key CHECK (pdf_key IS NULL OR btrim(pdf_key) <> ''),
    -- Target of the organization-consistent foreign keys below.
    ADD CONSTRAINT uq_measurement_results_id_org UNIQUE (id, organization_id);

-- 2. measurement_devices: model and active flag.
ALTER TABLE measurement_devices
    ADD COLUMN model      VARCHAR(64)  NULL,
    ADD COLUMN is_active  BOOLEAN      NOT NULL DEFAULT true,
    ADD CONSTRAINT uq_measurement_devices_id_org UNIQUE (id, organization_id);

-- A result links only a device of its own organization. Deleting a device
-- clears the link only (the organization column stays).
ALTER TABLE measurement_results
    ADD CONSTRAINT fk_measurement_results_device_org
        FOREIGN KEY (device_id, organization_id)
        REFERENCES measurement_devices (id, organization_id)
        ON DELETE SET NULL (device_id);

CREATE INDEX idx_measurement_results_device ON measurement_results (device_id) WHERE device_id IS NOT NULL;
CREATE INDEX idx_measurement_results_customer ON measurement_results (customer_user_id) WHERE customer_user_id IS NOT NULL;
CREATE INDEX idx_measurement_results_unparsed ON measurement_results (id) WHERE parsed_at IS NULL;

-- 3. measurement_values: one paint thickness reading (nexptg_report_measurements).
--    place_id: left | right | top | back (NexptgPlaceIdEnum).
--    part_type: panel code (NexptgPartTypeEnum, e.g. HOOD, LEFT_FRONT_DOOR).
--    is_inside: the reading is from the inner side (legacy is_inside).
--    position: order of the reading on the part (legacy position).
--    interpretation: NexptgInterpretationEnum value, -1..5
--      (-1 unknown, 0 too thin, 1 original, 2 second layer, 3 thin putty,
--       4 thick putty, 5 thick putty high); the code keeps the list.
CREATE TABLE measurement_values (
    id               BIGSERIAL PRIMARY KEY,
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    result_id        BIGINT        NOT NULL,
    place_id         VARCHAR(16)   NOT NULL,
    part_type        VARCHAR(32)   NOT NULL,
    is_inside        BOOLEAN       NOT NULL DEFAULT false,
    position         INT           NULL,
    value_um         NUMERIC(8,2)  NULL,
    interpretation   SMALLINT      NULL,
    substrate_type   VARCHAR(32)   NULL,
    measured_at      TIMESTAMPTZ   NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_measurement_values_result_org
        FOREIGN KEY (result_id, organization_id)
        REFERENCES measurement_results (id, organization_id) ON DELETE CASCADE,
    CONSTRAINT chk_measurement_values_place CHECK (btrim(place_id) <> ''),
    CONSTRAINT chk_measurement_values_part CHECK (btrim(part_type) <> ''),
    CONSTRAINT chk_measurement_values_value CHECK (value_um IS NULL OR value_um >= 0),
    CONSTRAINT chk_measurement_values_interpretation CHECK (interpretation IS NULL OR interpretation BETWEEN -1 AND 5)
);

CREATE INDEX idx_measurement_values_result ON measurement_values (result_id, is_inside, place_id);

CREATE TRIGGER trg_measurement_values_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON measurement_values
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. measurement_tires: one tire of a report (nexptg_report_tires).
--    section: tire position (legacy section); width/profile/diameter/maker/
--    season: size and make as reported; tread_depth_1/2_mm: the two tread
--    depth readings (legacy value1/value2).
CREATE TABLE measurement_tires (
    id               BIGSERIAL PRIMARY KEY,
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    result_id        BIGINT        NOT NULL,
    section          VARCHAR(32)   NULL,
    width            VARCHAR(16)   NULL,
    profile          VARCHAR(16)   NULL,
    diameter         VARCHAR(16)   NULL,
    maker            VARCHAR(64)   NULL,
    season           VARCHAR(32)   NULL,
    tread_depth_1_mm NUMERIC(5,2)  NULL,
    tread_depth_2_mm NUMERIC(5,2)  NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_measurement_tires_result_org
        FOREIGN KEY (result_id, organization_id)
        REFERENCES measurement_results (id, organization_id) ON DELETE CASCADE,
    CONSTRAINT chk_measurement_tires_depth CHECK (
        (tread_depth_1_mm IS NULL OR tread_depth_1_mm >= 0)
        AND (tread_depth_2_mm IS NULL OR tread_depth_2_mm >= 0)
    )
);

CREATE INDEX idx_measurement_tires_result ON measurement_tires (result_id);

CREATE TRIGGER trg_measurement_tires_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON measurement_tires
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 5. service_measurements: the before/after measurement of a service
--    (legacy service_nexptg_report). A service has at most one measurement
--    per phase and a measurement belongs to at most one service. The
--    measurement must be of the service's organization: the result side is
--    an organization-consistent foreign key, the service side a trigger.
CREATE TABLE service_measurements (
    id                     BIGSERIAL PRIMARY KEY,
    organization_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id               BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    service_id             BIGINT        NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    measurement_result_id  BIGINT        NOT NULL,
    phase                  VARCHAR(8)    NOT NULL,
    link_source            VARCHAR(8)    NOT NULL,
    confirmed_by           BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    confirmed_at           TIMESTAMPTZ   NULL,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_service_measurements_result_org
        FOREIGN KEY (measurement_result_id, organization_id)
        REFERENCES measurement_results (id, organization_id) ON DELETE CASCADE,
    CONSTRAINT uq_service_measurements_service_phase UNIQUE (service_id, phase),
    CONSTRAINT uq_service_measurements_result UNIQUE (measurement_result_id),
    CONSTRAINT chk_service_measurements_phase CHECK (phase IN ('before', 'after')),
    CONSTRAINT chk_service_measurements_link_source CHECK (link_source IN ('auto', 'manual')),
    -- A confirming user comes with the confirmation time.
    CONSTRAINT chk_service_measurements_confirmed CHECK (confirmed_by IS NULL OR confirmed_at IS NOT NULL)
);

CREATE FUNCTION service_measurements_check_service_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    svc_org BIGINT;
BEGIN
    SELECT organization_id INTO svc_org FROM services WHERE id = NEW.service_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF svc_org IS DISTINCT FROM NEW.organization_id THEN
        RAISE EXCEPTION 'service_measurements: service % belongs to organization %, not %',
            NEW.service_id, svc_org, NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_measurements_check_service_org
    BEFORE INSERT OR UPDATE OF organization_id, service_id ON service_measurements
    FOR EACH ROW
    EXECUTE FUNCTION service_measurements_check_service_org();

CREATE TRIGGER trg_service_measurements_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON service_measurements
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE TRIGGER trg_service_measurements_set_updated_at
    BEFORE UPDATE ON service_measurements
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 6. services: measurement check flag. The link lives in
--    service_measurements; the 000050 placeholder column stays (conservative)
--    but is no longer used.
ALTER TABLE services
    ADD COLUMN measurement_check_required  BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN measurement_checked_at      TIMESTAMPTZ  NULL;

COMMENT ON COLUMN services.measurement_result_id IS
    'Deprecated (TEC-293): not used; the measurement link is service_measurements.';

-- 7. Permissions. Source of truth: internal/platform/rbac/catalog.go
--    (appended last, in this order). Idempotent.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read measurements', 'measurements.read', 'measurements', ARRAY['managed', 'subtree']::text[], false, false,
       'Read paint thickness measurements, their readings, tires and service links (K28).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Link measurements', 'measurements.link', 'measurements', ARRAY['managed']::text[], false, false,
       'Link a measurement to a service as its before or after measurement and confirm the link (K28).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage measuring devices', 'measurement_devices.manage', 'measurements', ARRAY['managed']::text[], false, false,
       'Register, edit and deactivate the measuring devices of the active organization (K28).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'measurements.read', 'subtree'),
    ('super_admin', 'measurements.link', 'managed'),
    ('super_admin', 'measurement_devices.manage', 'managed'),
    ('center_staff', 'measurements.read', 'subtree'),
    ('center_staff', 'measurements.link', 'managed'),
    ('center_staff', 'measurement_devices.manage', 'managed'),
    ('distributor_owner', 'measurements.read', 'subtree'),
    ('distributor_owner', 'measurements.link', 'managed'),
    ('distributor_owner', 'measurement_devices.manage', 'managed'),
    ('distributor_staff', 'measurements.read', 'subtree'),
    ('distributor_staff', 'measurements.link', 'managed'),
    ('dealer_owner', 'measurements.read', 'managed'),
    ('dealer_owner', 'measurements.link', 'managed'),
    ('dealer_owner', 'measurement_devices.manage', 'managed'),
    ('dealer_staff', 'measurements.read', 'managed'),
    ('dealer_staff', 'measurements.link', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
