-- TEC-1229: stock view, quantity based min/max per warehouse and product,
-- warehouse low stock alerts and the stock count recount step.
--
-- 1. stock_reorder_rules: min / reorder point / max quantity of one product
--    in one warehouse (configuration, not a ledger). warning_days /
--    cover_days carry the stock forecast day thresholds per warehouse
--    (NULL: the organization's forecast threshold applies). alert_state is
--    the last evaluated state of the hourly low stock check (ok | low); a
--    notification goes out on the ok -> low edge only.
-- 2. stock_count_lines: a difference line can be sent back for a recount
--    (recount_status requested -> recounted); the first counted values are
--    kept. Approval may resolve a part of the lines (partial approval).
-- 3. Owner indexes on stock_movements for the warehouse movement list and
--    the end-of-day balances.

CREATE TABLE stock_reorder_rules (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL,
    brand_id            BIGINT       NOT NULL,
    warehouse_id        BIGINT       NOT NULL,
    product_id          BIGINT       NOT NULL,
    min_quantity        INT          NOT NULL DEFAULT 0,
    reorder_point       INT          NULL,
    max_quantity        INT          NULL,
    warning_days        INT          NULL,
    cover_days          INT          NULL,
    alert_state         VARCHAR(16)  NOT NULL DEFAULT 'ok',
    alerted_at          TIMESTAMPTZ  NULL,
    evaluated_at        TIMESTAMPTZ  NULL,
    created_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_reorder_rules_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_reorder_rules_warehouse_product UNIQUE (warehouse_id, product_id),
    CONSTRAINT fk_stock_reorder_rules_org FOREIGN KEY (organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_stock_reorder_rules_brand FOREIGN KEY (brand_id)
        REFERENCES brands (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_stock_reorder_rules_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_stock_reorder_rules_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_stock_reorder_rules_min CHECK (min_quantity >= 0),
    CONSTRAINT chk_stock_reorder_rules_point CHECK (reorder_point IS NULL OR reorder_point >= min_quantity),
    CONSTRAINT chk_stock_reorder_rules_max CHECK (
        max_quantity IS NULL OR (max_quantity >= min_quantity AND max_quantity >= COALESCE(reorder_point, 0))
    ),
    CONSTRAINT chk_stock_reorder_rules_some CHECK (
        min_quantity > 0 OR reorder_point IS NOT NULL OR max_quantity IS NOT NULL OR warning_days IS NOT NULL
    ),
    CONSTRAINT chk_stock_reorder_rules_warning CHECK (warning_days IS NULL OR warning_days > 0),
    CONSTRAINT chk_stock_reorder_rules_cover CHECK (cover_days IS NULL OR cover_days > 0),
    CONSTRAINT chk_stock_reorder_rules_alert CHECK (alert_state IN ('ok', 'low'))
);

CREATE INDEX idx_stock_reorder_rules_org ON stock_reorder_rules (organization_id, warehouse_id);
CREATE INDEX idx_stock_reorder_rules_product ON stock_reorder_rules (product_id);

CREATE TRIGGER trg_stock_reorder_rules_set_updated_at
    BEFORE UPDATE ON stock_reorder_rules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

ALTER TABLE stock_count_lines
    ADD COLUMN recount_status               VARCHAR(16)    NULL,
    ADD COLUMN recount_requested_at         TIMESTAMPTZ    NULL,
    ADD COLUMN recount_requested_by_user_id BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN recounted_at                 TIMESTAMPTZ    NULL,
    ADD COLUMN recounted_by_user_id         BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN recount_rounds               INT            NOT NULL DEFAULT 0,
    ADD COLUMN first_counted_quantity       INT            NULL,
    ADD COLUMN first_counted_meters         NUMERIC(10,2)  NULL,
    ADD COLUMN first_counted_location_id    BIGINT         NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    ADD COLUMN first_result                 VARCHAR(16)    NULL,
    ADD CONSTRAINT chk_stock_count_lines_recount CHECK (
        recount_status IS NULL OR recount_status IN ('requested', 'recounted')
    ),
    ADD CONSTRAINT chk_stock_count_lines_recount_rounds CHECK (recount_rounds >= 0);

CREATE INDEX idx_stock_movements_to_owner ON stock_movements (to_owner_type, to_owner_id, created_at);
CREATE INDEX idx_stock_movements_from_owner ON stock_movements (from_owner_type, from_owner_id, created_at);
