-- TEC-146 (F1-01c): the price a distributor sells a product to its dealers
-- (K8: the dealer's purchase price is the distributor's sale price). The
-- distributor writes its own rows; a dealer reads only the rows of its parent
-- distributor. Order-time rate freezing and the order price chain are TEC-96.

CREATE TABLE distributor_dealer_prices (
    id                  BIGSERIAL PRIMARY KEY,
    product_id          BIGINT         NOT NULL,
    brand_id            BIGINT         NOT NULL,
    distributor_org_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    currency            CHAR(3)        NOT NULL,
    price               NUMERIC(14,4)  NOT NULL,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_distributor_dealer_prices UNIQUE (product_id, distributor_org_id, currency),
    CONSTRAINT fk_distributor_dealer_prices_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_distributor_dealer_prices_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_distributor_dealer_prices_price CHECK (price >= 0)
);

CREATE INDEX idx_distributor_dealer_prices_org ON distributor_dealer_prices (distributor_org_id, product_id);
CREATE INDEX idx_distributor_dealer_prices_brand ON distributor_dealer_prices (brand_id, product_id);

CREATE TRIGGER trg_distributor_dealer_prices_set_updated_at
    BEFORE UPDATE ON distributor_dealer_prices
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The owner must be a distributor of the product's brand.
CREATE FUNCTION distributor_dealer_prices_check_org() RETURNS trigger
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

CREATE TRIGGER trg_distributor_dealer_prices_org
    BEFORE INSERT OR UPDATE OF distributor_org_id, brand_id ON distributor_dealer_prices
    FOR EACH ROW
    EXECUTE FUNCTION distributor_dealer_prices_check_org();
