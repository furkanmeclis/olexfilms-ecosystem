-- TEC-483 (F5-04a): stock forecast schema, permissions and sqlc.
-- The worker/API arrive in F5-04b/c. This migration only creates the
-- daily forecast snapshots, threshold overrides and center network-demand
-- snapshots required by the algorithmic (non-AI, K15) stock forecast.

CREATE TABLE stock_forecasts (
    id                        BIGSERIAL      PRIMARY KEY,
    uuid                      UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id           BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                  BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id                BIGINT         NOT NULL,
    computed_on               DATE           NOT NULL,
    is_latest                 BOOLEAN        NOT NULL DEFAULT false,
    on_hand_qty               INT            NOT NULL DEFAULT 0,
    on_hand_meters            NUMERIC(14,2)  NOT NULL DEFAULT 0,
    avg_daily_30              NUMERIC(14,4)  NOT NULL DEFAULT 0,
    avg_daily_90              NUMERIC(14,4)  NOT NULL DEFAULT 0,
    seasonality_factor        NUMERIC(6,3)   NOT NULL DEFAULT 1.000,
    avg_meters_per_vehicle    NUMERIC(14,4)  NULL,
    vehicles_left             NUMERIC(14,2)  NULL,
    days_left                 NUMERIC(10,2)  NULL,
    depletion_date            DATE           NULL,
    data_days                 INT            NOT NULL DEFAULT 0,
    status                    VARCHAR(32)    NOT NULL,
    suggested_qty             INT            NULL,
    suggested_meters          NUMERIC(14,2)  NULL,
    created_at                TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_forecasts_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_forecasts_org_product_day UNIQUE (organization_id, product_id, computed_on),
    CONSTRAINT fk_stock_forecasts_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_forecasts_on_hand_qty CHECK (on_hand_qty >= 0),
    CONSTRAINT chk_stock_forecasts_on_hand_meters CHECK (on_hand_meters >= 0),
    CONSTRAINT chk_stock_forecasts_avg_daily_30 CHECK (avg_daily_30 >= 0),
    CONSTRAINT chk_stock_forecasts_avg_daily_90 CHECK (avg_daily_90 >= 0),
    CONSTRAINT chk_stock_forecasts_seasonality CHECK (seasonality_factor > 0),
    CONSTRAINT chk_stock_forecasts_avg_meters_vehicle CHECK (
        avg_meters_per_vehicle IS NULL OR avg_meters_per_vehicle > 0),
    CONSTRAINT chk_stock_forecasts_vehicles_left CHECK (vehicles_left IS NULL OR vehicles_left >= 0),
    CONSTRAINT chk_stock_forecasts_days_left CHECK (days_left IS NULL OR days_left >= 0),
    CONSTRAINT chk_stock_forecasts_data_days CHECK (data_days >= 0),
    CONSTRAINT chk_stock_forecasts_status CHECK (
        status IN ('insufficient_data', 'ok', 'warning', 'critical', 'no_consumption')),
    CONSTRAINT chk_stock_forecasts_suggested_qty CHECK (suggested_qty IS NULL OR suggested_qty >= 0),
    CONSTRAINT chk_stock_forecasts_suggested_meters CHECK (suggested_meters IS NULL OR suggested_meters >= 0)
);

CREATE UNIQUE INDEX uq_stock_forecasts_latest
    ON stock_forecasts (organization_id, product_id)
    WHERE is_latest;
CREATE INDEX idx_stock_forecasts_org_latest
    ON stock_forecasts (organization_id, is_latest, days_left, id);
CREATE INDEX idx_stock_forecasts_brand_product_day
    ON stock_forecasts (brand_id, product_id, computed_on DESC);
CREATE INDEX idx_stock_forecasts_status
    ON stock_forecasts (organization_id, status, days_left, id)
    WHERE is_latest;

CREATE TRIGGER trg_stock_forecasts_set_updated_at
    BEFORE UPDATE ON stock_forecasts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_stock_forecasts_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON stock_forecasts
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE TABLE stock_forecast_thresholds (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id       BIGINT        NULL,
    warning_days     INT           NOT NULL,
    cover_days       INT           NOT NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_forecast_thresholds_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_forecast_thresholds_org_product UNIQUE NULLS NOT DISTINCT (organization_id, product_id),
    CONSTRAINT fk_stock_forecast_thresholds_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_forecast_thresholds_warning CHECK (warning_days > 0),
    CONSTRAINT chk_stock_forecast_thresholds_cover CHECK (cover_days > 0)
);

CREATE INDEX idx_stock_forecast_thresholds_brand_product
    ON stock_forecast_thresholds (brand_id, product_id)
    WHERE product_id IS NOT NULL;

CREATE TRIGGER trg_stock_forecast_thresholds_set_updated_at
    BEFORE UPDATE ON stock_forecast_thresholds
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_stock_forecast_thresholds_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON stock_forecast_thresholds
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE TABLE network_demand_forecasts (
    id                         BIGSERIAL      PRIMARY KEY,
    uuid                       UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id            BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                   BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id                 BIGINT         NOT NULL,
    forecast_month             DATE           NOT NULL,
    expected_qty               INT            NOT NULL DEFAULT 0,
    expected_meters            NUMERIC(14,2)  NOT NULL DEFAULT 0,
    network_on_hand_qty        INT            NOT NULL DEFAULT 0,
    network_on_hand_meters     NUMERIC(14,2)  NOT NULL DEFAULT 0,
    open_order_qty             INT            NOT NULL DEFAULT 0,
    open_order_meters          NUMERIC(14,2)  NOT NULL DEFAULT 0,
    suggested_production_qty   INT            NOT NULL DEFAULT 0,
    suggested_production_meters NUMERIC(14,2) NOT NULL DEFAULT 0,
    computed_at                TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    created_at                 TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_network_demand_forecasts_uuid UNIQUE (uuid),
    CONSTRAINT uq_network_demand_forecasts_product_month UNIQUE (brand_id, product_id, forecast_month),
    CONSTRAINT fk_network_demand_forecasts_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_network_demand_forecasts_month CHECK (forecast_month = date_trunc('month', forecast_month)::date),
    CONSTRAINT chk_network_demand_forecasts_expected_qty CHECK (expected_qty >= 0),
    CONSTRAINT chk_network_demand_forecasts_expected_meters CHECK (expected_meters >= 0),
    CONSTRAINT chk_network_demand_forecasts_on_hand_qty CHECK (network_on_hand_qty >= 0),
    CONSTRAINT chk_network_demand_forecasts_on_hand_meters CHECK (network_on_hand_meters >= 0),
    CONSTRAINT chk_network_demand_forecasts_open_qty CHECK (open_order_qty >= 0),
    CONSTRAINT chk_network_demand_forecasts_open_meters CHECK (open_order_meters >= 0),
    CONSTRAINT chk_network_demand_forecasts_suggested_qty CHECK (suggested_production_qty >= 0),
    CONSTRAINT chk_network_demand_forecasts_suggested_meters CHECK (suggested_production_meters >= 0)
);

CREATE INDEX idx_network_demand_forecasts_brand_month
    ON network_demand_forecasts (brand_id, forecast_month, product_id);

CREATE TRIGGER trg_network_demand_forecasts_set_updated_at
    BEFORE UPDATE ON network_demand_forecasts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_network_demand_forecasts_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON network_demand_forecasts
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read stock forecasts', 'stock_forecast.read', 'stock_forecast',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read stock forecast snapshots and product history.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage stock forecast thresholds', 'stock_forecast.manage', 'stock_forecast',
       ARRAY['managed', 'brand', 'all']::text[], false, false,
       'Edit stock forecast warning and cover thresholds.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read network stock demand forecasts', 'stock_forecast.network.read', 'stock_forecast',
       ARRAY['brand', 'all']::text[], false, false,
       'Read center network demand forecasts for the brand.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'stock_forecast.read', 'all'),
    ('super_admin', 'stock_forecast.manage', 'all'),
    ('super_admin', 'stock_forecast.network.read', 'all'),
    ('center_staff', 'stock_forecast.read', 'brand'),
    ('center_staff', 'stock_forecast.manage', 'brand'),
    ('center_staff', 'stock_forecast.network.read', 'brand'),
    ('center_warehouse', 'stock_forecast.read', 'brand'),
    ('center_warehouse', 'stock_forecast.manage', 'brand'),
    ('center_warehouse', 'stock_forecast.network.read', 'brand'),
    ('distributor_owner', 'stock_forecast.read', 'subtree'),
    ('distributor_owner', 'stock_forecast.manage', 'managed'),
    ('distributor_staff', 'stock_forecast.read', 'subtree'),
    ('distributor_warehouse_staff', 'stock_forecast.read', 'subtree'),
    ('dealer_owner', 'stock_forecast.read', 'managed'),
    ('dealer_owner', 'stock_forecast.manage', 'managed'),
    ('dealer_staff', 'stock_forecast.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
