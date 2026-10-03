-- TEC-266 (F2-02a): Glorian sync schema (K2, K20).
--
-- Glorian stays on its own install; the warehouse keeps the brand=glorian
-- stock and syncs with the Glorian hub through the Inventory API (catalog
-- and dealer pull, barcode push, order outbound, reconcile). Every row
-- carries the center organization and the glorian brand. Child tables pin
-- their organization_id/brand_id to the connection's with a composite
-- foreign key, so a row can never point at another brand's connection.
--
-- The API key is stored only encrypted (api_key_enc) with the application
-- secret box (crypto.SecretBox, APP_ENCRYPTION_KEY), like the other
-- *_enc secret columns; it is never written in plain text.

-- 1. Connections (key = glorian, unique per brand).
CREATE TABLE integration_connections (
    id                    BIGSERIAL PRIMARY KEY,
    uuid                  UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    key                   VARCHAR(32)   NOT NULL,
    base_url              VARCHAR(500)  NOT NULL,
    api_key_enc           TEXT          NOT NULL DEFAULT '',
    default_warehouse_id  BIGINT        NULL,
    active                BOOLEAN       NOT NULL DEFAULT false,
    api_version           VARCHAR(8)    NOT NULL DEFAULT '1',
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_integration_connections_uuid UNIQUE (uuid),
    CONSTRAINT uq_integration_connections_brand_key UNIQUE (brand_id, key),
    CONSTRAINT uq_integration_connections_id_org_brand UNIQUE (id, organization_id, brand_id),
    -- The default warehouse belongs to the connection's organization.
    CONSTRAINT fk_integration_connections_warehouse FOREIGN KEY (default_warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_integration_connections_key CHECK (key ~ '^[a-z0-9_]{1,32}$'),
    CONSTRAINT chk_integration_connections_base_url CHECK (base_url ~ '^https?://'),
    CONSTRAINT chk_integration_connections_api_version CHECK (api_version ~ '^[0-9]{1,8}$'),
    -- An active connection must have a key to call with.
    CONSTRAINT chk_integration_connections_active_key CHECK (NOT active OR api_key_enc <> '')
);

CREATE TRIGGER trg_integration_connections_set_updated_at
    BEFORE UPDATE ON integration_connections
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- brand_id must be the brand of the organization (000048 helper).
CREATE TRIGGER trg_integration_connections_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON integration_connections
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 2. Local warehouse location <-> remote location code.
CREATE TABLE connection_location_maps (
    id                     BIGSERIAL PRIMARY KEY,
    organization_id        BIGINT        NOT NULL,
    brand_id               BIGINT        NOT NULL,
    connection_id          BIGINT        NOT NULL,
    warehouse_location_id  BIGINT        NOT NULL,
    remote_location_code   VARCHAR(64)   NOT NULL,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_connection_location_maps_connection FOREIGN KEY (connection_id, organization_id, brand_id)
        REFERENCES integration_connections (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_connection_location_maps_location FOREIGN KEY (warehouse_location_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT uq_connection_location_maps_location UNIQUE (connection_id, warehouse_location_id),
    CONSTRAINT uq_connection_location_maps_remote UNIQUE (connection_id, remote_location_code),
    CONSTRAINT chk_connection_location_maps_remote CHECK (btrim(remote_location_code) <> '')
);

CREATE INDEX idx_connection_location_maps_location ON connection_location_maps (warehouse_location_id);

CREATE TRIGGER trg_connection_location_maps_set_updated_at
    BEFORE UPDATE ON connection_location_maps
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 3. Sync runs. watermark is the updated_since cursor of the next
-- incremental pull; counts holds per-run totals (fetched, created, ...).
CREATE TABLE integration_sync_runs (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    connection_id    BIGINT        NOT NULL,
    kind             VARCHAR(32)   NOT NULL,
    status           VARCHAR(16)   NOT NULL DEFAULT 'running',
    started_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ   NULL,
    watermark        TIMESTAMPTZ   NULL,
    counts           JSONB         NOT NULL DEFAULT '{}'::jsonb,
    error            TEXT          NULL,
    CONSTRAINT uq_integration_sync_runs_uuid UNIQUE (uuid),
    CONSTRAINT fk_integration_sync_runs_connection FOREIGN KEY (connection_id, organization_id, brand_id)
        REFERENCES integration_connections (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_integration_sync_runs_kind CHECK (kind IN (
        'pull_categories', 'pull_products', 'pull_dealers', 'pull_stock',
        'push_barcodes', 'outbound', 'reconcile')),
    CONSTRAINT chk_integration_sync_runs_status CHECK (status IN ('running', 'succeeded', 'failed')),
    CONSTRAINT chk_integration_sync_runs_finished CHECK ((status = 'running') = (finished_at IS NULL)),
    CONSTRAINT chk_integration_sync_runs_counts CHECK (jsonb_typeof(counts) = 'object')
);

CREATE INDEX idx_integration_sync_runs_connection_kind
    ON integration_sync_runs (connection_id, kind, started_at DESC);
CREATE INDEX idx_integration_sync_runs_org_started ON integration_sync_runs (organization_id, started_at);

-- 4. External parties: Glorian dealers pulled from the hub (a counterparty
-- record like customers; not users of this app, K2).
CREATE TABLE integration_external_parties (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    connection_id    BIGINT        NOT NULL,
    remote_id        VARCHAR(64)   NOT NULL,
    name             VARCHAR(200)  NOT NULL,
    phone_e164       VARCHAR(16)   NULL,
    active           BOOLEAN       NOT NULL DEFAULT true,
    synced_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_integration_external_parties_uuid UNIQUE (uuid),
    CONSTRAINT uq_integration_external_parties_remote UNIQUE (connection_id, remote_id),
    CONSTRAINT fk_integration_external_parties_connection FOREIGN KEY (connection_id, organization_id, brand_id)
        REFERENCES integration_connections (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_integration_external_parties_remote CHECK (btrim(remote_id) <> ''),
    CONSTRAINT chk_integration_external_parties_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_integration_external_parties_phone CHECK (phone_e164 IS NULL OR phone_e164 ~ '^\+[1-9][0-9]{6,14}$')
);

CREATE INDEX idx_integration_external_parties_phone ON integration_external_parties (phone_e164)
    WHERE phone_e164 IS NOT NULL;

CREATE TRIGGER trg_integration_external_parties_set_updated_at
    BEFORE UPDATE ON integration_external_parties
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 5. Order outbound: an order sent to the hub (external_reference is the
-- idempotency key). held means it waits for a reason in held_reason.
CREATE TABLE order_outbounds (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL,
    brand_id            BIGINT        NOT NULL,
    order_id            BIGINT        NOT NULL REFERENCES orders (id) ON DELETE RESTRICT,
    connection_id       BIGINT        NOT NULL,
    external_reference  VARCHAR(64)   NOT NULL,
    state               VARCHAR(16)   NOT NULL DEFAULT 'pending',
    held_reason         VARCHAR(32)   NULL,
    attempts            INTEGER       NOT NULL DEFAULT 0,
    last_error          TEXT          NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_order_outbounds_uuid UNIQUE (uuid),
    CONSTRAINT uq_order_outbounds_order_connection UNIQUE (order_id, connection_id),
    CONSTRAINT uq_order_outbounds_reference UNIQUE (connection_id, external_reference),
    CONSTRAINT fk_order_outbounds_connection FOREIGN KEY (connection_id, organization_id, brand_id)
        REFERENCES integration_connections (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_order_outbounds_reference CHECK (btrim(external_reference) <> ''),
    CONSTRAINT chk_order_outbounds_state CHECK (state IN ('pending', 'held', 'sent', 'failed', 'cancelled')),
    CONSTRAINT chk_order_outbounds_held_reason CHECK (
        held_reason IS NULL OR held_reason IN ('missing_customer_link', 'inactive_connection')),
    CONSTRAINT chk_order_outbounds_held CHECK ((state = 'held') = (held_reason IS NOT NULL)),
    CONSTRAINT chk_order_outbounds_attempts CHECK (attempts >= 0)
);

CREATE INDEX idx_order_outbounds_connection_state ON order_outbounds (connection_id, state);

CREATE TRIGGER trg_order_outbounds_set_updated_at
    BEFORE UPDATE ON order_outbounds
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The order belongs to the connection's brand (K1/K20).
CREATE FUNCTION order_outbounds_check_order() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    order_brand BIGINT;
BEGIN
    SELECT brand_id INTO order_brand FROM orders WHERE id = NEW.order_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF order_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'order_outbounds: order % (brand %) does not match brand %',
            NEW.order_id, order_brand, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_order_outbounds_check_order
    BEFORE INSERT OR UPDATE OF order_id, brand_id ON order_outbounds
    FOR EACH ROW
    EXECUTE FUNCTION order_outbounds_check_order();

-- 6. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last). super_admin only for now; center roles get them with
-- the Glorian settings screen.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'View Glorian integration', 'integrations.glorian.view', 'integrations', ARRAY['brand', 'all']::text[], false, false,
       'Glorian hub connection, location map, sync runs, dealers and order outbound state (K2).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage Glorian integration', 'integrations.glorian.manage', 'integrations', ARRAY['brand', 'all']::text[], true, false,
       'Edit the Glorian hub connection and API key, location map; start syncs and retry outbound orders.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'integrations.glorian.view', 'all'),
    ('super_admin', 'integrations.glorian.manage', 'all')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
