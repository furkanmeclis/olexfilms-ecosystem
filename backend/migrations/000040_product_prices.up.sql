-- TEC-144 (F1-01a): center price list and distributor-specific prices (K8).
-- Visibility is enforced by the pricing.* permissions in the use case layer.
-- brand_id is denormalised from the product and kept consistent through the
-- (product_id, brand_id) foreign key.

CREATE TABLE product_prices (
    id                         BIGSERIAL PRIMARY KEY,
    product_id                 BIGINT         NOT NULL,
    brand_id                   BIGINT         NOT NULL,
    currency                   CHAR(3)        NOT NULL,
    purchase_price             NUMERIC(14,4)  NULL,
    sale_to_distributor_price  NUMERIC(14,4)  NULL,
    recommended_sale_price     NUMERIC(14,4)  NULL,
    created_at                 TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_prices_product_currency UNIQUE (product_id, currency),
    CONSTRAINT fk_product_prices_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_product_prices_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_product_prices_purchase CHECK (purchase_price IS NULL OR purchase_price >= 0),
    CONSTRAINT chk_product_prices_sale CHECK (sale_to_distributor_price IS NULL OR sale_to_distributor_price >= 0),
    CONSTRAINT chk_product_prices_recommended CHECK (recommended_sale_price IS NULL OR recommended_sale_price >= 0)
);

CREATE INDEX idx_product_prices_brand ON product_prices (brand_id, product_id);

CREATE TRIGGER trg_product_prices_set_updated_at
    BEFORE UPDATE ON product_prices
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE distributor_price_overrides (
    id                  BIGSERIAL PRIMARY KEY,
    product_id          BIGINT         NOT NULL,
    brand_id            BIGINT         NOT NULL,
    distributor_org_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    currency            CHAR(3)        NOT NULL,
    price               NUMERIC(14,4)  NOT NULL,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_distributor_price_overrides UNIQUE (product_id, distributor_org_id, currency),
    CONSTRAINT fk_distributor_price_overrides_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_distributor_price_overrides_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_distributor_price_overrides_price CHECK (price >= 0)
);

CREATE INDEX idx_distributor_price_overrides_org ON distributor_price_overrides (distributor_org_id, product_id);

CREATE TRIGGER trg_distributor_price_overrides_set_updated_at
    BEFORE UPDATE ON distributor_price_overrides
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The override target must be a distributor of the product's brand.
CREATE FUNCTION distributor_price_overrides_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type  TEXT;
    org_brand BIGINT;
BEGIN
    SELECT type, brand_id INTO org_type, org_brand
    FROM organizations WHERE id = NEW.distributor_org_id;
    IF org_type IS DISTINCT FROM 'distributor' OR org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'organization % is not a distributor of brand %',
            NEW.distributor_org_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_distributor_price_overrides_org
    BEFORE INSERT OR UPDATE OF distributor_org_id, brand_id ON distributor_price_overrides
    FOR EACH ROW
    EXECUTE FUNCTION distributor_price_overrides_check_org();
