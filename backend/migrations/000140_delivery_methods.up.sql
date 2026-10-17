-- TEC-768 (Depo F4.3): delivery methods and the shipping info of an order.
--
--   * delivery_methods: the seller organization's catalog of carriers
--     (name, code, tracking URL template with a {tracking_no} placeholder,
--     active, sort order). Every selling organization has one system method
--     "warehouse_pickup" (Ambar teslim) that cannot be edited or deleted; the
--     use case creates it on first use (one per organization, partial unique
--     index below).
--   * orders.delivery_method_id: the method chosen when the order ships. It
--     belongs to the seller (organization_id = seller_org_id), so the FK pins
--     the method to the order's organization. delivery_mode stays the coarse
--     hand|warehouse marker (warehouse for the system method).
--
-- brand_id follows the organization (AGENTS.md: every business table carries
-- organization_id + brand_id); the trigger keeps them consistent.

CREATE TABLE delivery_methods (
    id                     BIGSERIAL PRIMARY KEY,
    uuid                   UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id               BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    code                   VARCHAR(32)   NOT NULL,
    name                   VARCHAR(120)  NOT NULL,
    tracking_url_template  VARCHAR(500)  NULL,
    active                 BOOLEAN       NOT NULL DEFAULT true,
    sort_order             INTEGER       NOT NULL DEFAULT 0,
    is_system              BOOLEAN       NOT NULL DEFAULT false,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_delivery_methods_uuid UNIQUE (uuid),
    CONSTRAINT uq_delivery_methods_org_code UNIQUE (organization_id, code),
    -- Target of fk_orders_delivery_method (method of the seller).
    CONSTRAINT uq_delivery_methods_id_org UNIQUE (id, organization_id),
    CONSTRAINT chk_delivery_methods_code CHECK (code ~ '^[a-z0-9_]{1,32}$'),
    CONSTRAINT chk_delivery_methods_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_delivery_methods_template CHECK (
        tracking_url_template IS NULL OR btrim(tracking_url_template) <> ''
    ),
    CONSTRAINT chk_delivery_methods_sort_order CHECK (sort_order BETWEEN 0 AND 100000)
);

-- One system method per organization.
CREATE UNIQUE INDEX uq_delivery_methods_org_system
    ON delivery_methods (organization_id) WHERE is_system;
CREATE INDEX idx_delivery_methods_org_sort ON delivery_methods (organization_id, sort_order, id);

CREATE TRIGGER trg_delivery_methods_set_updated_at
    BEFORE UPDATE ON delivery_methods
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- brand_id is the organization's brand and never changes.
CREATE FUNCTION delivery_methods_check_brand() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand BIGINT;
BEGIN
    SELECT brand_id INTO org_brand FROM organizations WHERE id = NEW.organization_id;
    IF org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'delivery method brand % must be the brand of organization %',
            NEW.brand_id, NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_delivery_methods_check_brand
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON delivery_methods
    FOR EACH ROW
    EXECUTE FUNCTION delivery_methods_check_brand();

ALTER TABLE orders
    ADD COLUMN delivery_method_id BIGINT NULL;

ALTER TABLE orders
    ADD CONSTRAINT fk_orders_delivery_method FOREIGN KEY (delivery_method_id, organization_id)
        REFERENCES delivery_methods (id, organization_id) ON DELETE RESTRICT;

CREATE INDEX idx_orders_delivery_method ON orders (delivery_method_id) WHERE delivery_method_id IS NOT NULL;
