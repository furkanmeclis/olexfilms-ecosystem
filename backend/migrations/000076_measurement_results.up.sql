-- TEC-233 (F2-05a): minimal measurement storage (K28).
--
-- POST /v1/mobile/measurements stores the upload as a raw record so the
-- mobile measurement flow keeps working through the F2 cut-over. raw holds
-- the request body as received (the NexPTG report sits under raw.raw).
-- F3-02 (TEC-113) extends this table: mandatory VIN, device registry link,
-- before/after pairing, difference table and PDF.
--
-- status: accepted (VIN present) | vin_pending ("tamamlanacak", VIN missing).
-- source: mobile (the API) | legacy_import (the migrator, TEC-262).
-- The idempotency key (Idempotency-Key header) and client_measurement_id
-- are unique per organization; a repeat returns the first row.

CREATE TABLE measurement_results (
    id                     BIGSERIAL PRIMARY KEY,
    uuid                   UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id               BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    service_id             BIGINT        NULL REFERENCES services (id) ON DELETE SET NULL,
    vehicle_id             BIGINT        NULL REFERENCES vehicles (id) ON DELETE SET NULL,
    vin                    VARCHAR(17)   NULL,
    status                 VARCHAR(16)   NOT NULL,
    raw                    JSONB         NOT NULL,
    client_measurement_id  VARCHAR(128)  NULL,
    idempotency_key        VARCHAR(128)  NULL,
    device_serial          VARCHAR(64)   NULL,
    source                 VARCHAR(16)   NOT NULL DEFAULT 'mobile',
    created_by             BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_measurement_results_uuid UNIQUE (uuid),
    CONSTRAINT chk_measurement_results_status CHECK (status IN ('accepted', 'vin_pending')),
    CONSTRAINT chk_measurement_results_source CHECK (source IN ('mobile', 'legacy_import')),
    -- accepted means a VIN is on the row; vin_pending means it is not.
    CONSTRAINT chk_measurement_results_vin_status CHECK ((status = 'accepted') = (vin IS NOT NULL)),
    CONSTRAINT chk_measurement_results_vin CHECK (vin IS NULL OR btrim(vin) <> ''),
    CONSTRAINT chk_measurement_results_raw CHECK (jsonb_typeof(raw) = 'object'),
    -- Uploads through the API always carry the uploading user.
    CONSTRAINT chk_measurement_results_created_by CHECK (source = 'legacy_import' OR created_by IS NOT NULL)
);

CREATE UNIQUE INDEX uq_measurement_results_idempotency_key
    ON measurement_results (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX uq_measurement_results_client_id
    ON measurement_results (organization_id, client_measurement_id)
    WHERE client_measurement_id IS NOT NULL;
CREATE INDEX idx_measurement_results_org_created ON measurement_results (organization_id, created_at DESC);
CREATE INDEX idx_measurement_results_service ON measurement_results (service_id) WHERE service_id IS NOT NULL;
CREATE INDEX idx_measurement_results_vin ON measurement_results (brand_id, vin) WHERE vin IS NOT NULL;

-- brand_id must be the brand of the organization (000048 helper).
CREATE TRIGGER trg_measurement_results_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON measurement_results
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Measuring devices (NexPTG serials) of an organization; the migrator
-- (TEC-262) fills it from the legacy device records, F3-02 links results.
CREATE TABLE measurement_devices (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    serial           VARCHAR(64)   NOT NULL,
    label            VARCHAR(255)  NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_measurement_devices_uuid UNIQUE (uuid),
    CONSTRAINT uq_measurement_devices_org_serial UNIQUE (organization_id, serial),
    CONSTRAINT chk_measurement_devices_serial CHECK (btrim(serial) <> '')
);

CREATE TRIGGER trg_measurement_devices_set_updated_at
    BEFORE UPDATE ON measurement_devices
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_measurement_devices_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON measurement_devices
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Permission. Source of truth: internal/platform/rbac/catalog.go (appended
-- last). Roles that write services upload measurements into their own
-- organization (managed); super_admin too.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Upload measurements', 'measurements.write', 'measurements', ARRAY['managed']::text[], false, false,
       'Upload paint thickness measurements from the mobile app into the active organization (K28).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'measurements.write', 'managed'),
    ('center_staff', 'measurements.write', 'managed'),
    ('distributor_owner', 'measurements.write', 'managed'),
    ('distributor_staff', 'measurements.write', 'managed'),
    ('dealer_owner', 'measurements.write', 'managed'),
    ('dealer_staff', 'measurements.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
