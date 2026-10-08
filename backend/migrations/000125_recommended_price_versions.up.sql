-- TEC-505 (F5-09a): recommended sale price versions (TEC-134, K8).
--
--   * recommended_price_versions: append-only ledger of the brand center's
--     recommended end-customer prices per product x country x currency.
--     country_id NULL is the currency-wide price. A version takes effect on
--     effective_from; superseded_at is the only column an UPDATE may touch
--     (set once, when a later version replaced it). Rows are never deleted
--     directly (only the product cascade removes them).
--   * recommended_prices_current: projection product x country x currency ->
--     the version in force, its price and effective date.
--   * product_prices.recommended_sale_price (F1-01) stays in sync with the
--     country-wide (country_id NULL) current row in both directions: a
--     current row change writes the column, and a direct write of the column
--     (the existing price list API) is recorded as a 'price_list' version
--     that takes effect today. Orders keep reading the snapshot from
--     product_prices. Values compare at 2 decimals, so a 4-decimal legacy
--     value is never rewritten by the sync.
--   * price_discipline_snapshots: daily end-customer price of each dealer /
--     serving distributor (dealer_product_prices) against the recommended
--     price, with the deviation and the realised average sale price of the
--     last 30 days (product_sales), filled by the F5-09b worker.
--
-- Existing recommended prices are carried over as 'migration' versions
-- effective on the migration day.

-- 1. Versions ------------------------------------------------------------------
CREATE TABLE recommended_price_versions (
    id                    BIGSERIAL      PRIMARY KEY,
    uuid                  UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id            BIGINT         NOT NULL,
    country_id            BIGINT         NULL REFERENCES countries (id) ON DELETE RESTRICT,
    currency              CHAR(3)        NOT NULL,
    price                 NUMERIC(18,2)  NOT NULL,
    effective_from        DATE           NOT NULL,
    source                VARCHAR(16)    NOT NULL DEFAULT 'publish',
    published_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    published_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    batch_id              UUID           NULL,
    note                  TEXT           NOT NULL DEFAULT '',
    superseded_at         TIMESTAMPTZ    NULL,
    CONSTRAINT uq_recommended_price_versions_uuid UNIQUE (uuid),
    CONSTRAINT fk_recommended_price_versions_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_recommended_price_versions_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_recommended_price_versions_price CHECK (price >= 0),
    CONSTRAINT chk_recommended_price_versions_source CHECK (source IN ('publish', 'price_list', 'migration')),
    CONSTRAINT chk_recommended_price_versions_note CHECK (char_length(note) <= 2000),
    CONSTRAINT chk_recommended_price_versions_superseded CHECK (superseded_at IS NULL OR superseded_at >= published_at)
);

-- One live (not superseded) version per key and effective day; a republish
-- for the same day supersedes the previous one first.
CREATE UNIQUE INDEX uq_recommended_price_versions_live
    ON recommended_price_versions (product_id, country_id, currency, effective_from)
    NULLS NOT DISTINCT
    WHERE superseded_at IS NULL;
CREATE INDEX idx_recommended_price_versions_brand
    ON recommended_price_versions (brand_id, effective_from DESC, id DESC);
CREATE INDEX idx_recommended_price_versions_due
    ON recommended_price_versions (effective_from)
    WHERE superseded_at IS NULL;
CREATE INDEX idx_recommended_price_versions_batch
    ON recommended_price_versions (batch_id)
    WHERE batch_id IS NOT NULL;
CREATE INDEX idx_recommended_price_versions_country
    ON recommended_price_versions (country_id)
    WHERE country_id IS NOT NULL;
CREATE INDEX idx_recommended_price_versions_publisher
    ON recommended_price_versions (published_by_user_id)
    WHERE published_by_user_id IS NOT NULL;

-- The owner is the brand center.
CREATE FUNCTION recommended_price_versions_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type  TEXT;
    org_brand BIGINT;
BEGIN
    SELECT type, brand_id INTO org_type, org_brand
    FROM organizations WHERE id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_type IS DISTINCT FROM 'center' OR org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'recommended_price_versions: organization % is not the center of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_recommended_price_versions_org
    BEFORE INSERT ON recommended_price_versions
    FOR EACH ROW
    EXECUTE FUNCTION recommended_price_versions_check_org();

-- Append-only: an UPDATE may only set superseded_at once; a direct DELETE
-- is refused (pg_trigger_depth() > 1 is the product cascade).
CREATE FUNCTION recommended_price_versions_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'recommended_price_versions is append-only'
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.superseded_at IS NOT NULL
       OR NEW.superseded_at IS NULL
       OR (to_jsonb(NEW) - 'superseded_at') IS DISTINCT FROM (to_jsonb(OLD) - 'superseded_at') THEN
        RAISE EXCEPTION 'recommended_price_versions is append-only (only superseded_at may be set, once)'
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_recommended_price_versions_append_only
    BEFORE UPDATE OR DELETE ON recommended_price_versions
    FOR EACH ROW
    EXECUTE FUNCTION recommended_price_versions_append_only();

-- 2. Current projection ---------------------------------------------------------
CREATE TABLE recommended_prices_current (
    id               BIGSERIAL      PRIMARY KEY,
    organization_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id       BIGINT         NOT NULL,
    country_id       BIGINT         NULL REFERENCES countries (id) ON DELETE RESTRICT,
    currency         CHAR(3)        NOT NULL,
    version_id       BIGINT         NOT NULL REFERENCES recommended_price_versions (id) ON DELETE CASCADE,
    price            NUMERIC(18,2)  NOT NULL,
    effective_from   DATE           NOT NULL,
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_recommended_prices_current_key UNIQUE NULLS NOT DISTINCT (product_id, country_id, currency),
    CONSTRAINT uq_recommended_prices_current_version UNIQUE (version_id),
    CONSTRAINT fk_recommended_prices_current_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_recommended_prices_current_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_recommended_prices_current_price CHECK (price >= 0)
);

CREATE INDEX idx_recommended_prices_current_brand ON recommended_prices_current (brand_id, product_id);
CREATE INDEX idx_recommended_prices_current_country ON recommended_prices_current (country_id)
    WHERE country_id IS NOT NULL;

-- A current row mirrors its version.
CREATE FUNCTION recommended_prices_current_check_version() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v recommended_price_versions%ROWTYPE;
BEGIN
    SELECT * INTO v FROM recommended_price_versions WHERE id = NEW.version_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF v.product_id <> NEW.product_id OR v.brand_id <> NEW.brand_id
       OR v.organization_id <> NEW.organization_id
       OR v.country_id IS DISTINCT FROM NEW.country_id OR v.currency <> NEW.currency
       OR v.price <> NEW.price OR v.effective_from <> NEW.effective_from THEN
        RAISE EXCEPTION 'recommended_prices_current: row does not match version %', NEW.version_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_recommended_prices_current_version
    BEFORE INSERT OR UPDATE ON recommended_prices_current
    FOR EACH ROW
    EXECUTE FUNCTION recommended_prices_current_check_version();

CREATE TRIGGER trg_recommended_prices_current_set_updated_at
    BEFORE UPDATE ON recommended_prices_current
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Makes a version the one in force for its key: every older live version of
-- the key (the previous current one and stale schedules) is superseded and
-- the projection points to the version. Used by publication, the
-- effective-date job (F5-09b), the price list sync and this migration.
CREATE FUNCTION recommended_prices_make_current(p_version_id BIGINT) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    v recommended_price_versions%ROWTYPE;
BEGIN
    SELECT * INTO v FROM recommended_price_versions WHERE id = p_version_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'recommended price version % does not exist', p_version_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF v.superseded_at IS NOT NULL THEN
        RAISE EXCEPTION 'recommended price version % is superseded', p_version_id
            USING ERRCODE = 'check_violation';
    END IF;
    UPDATE recommended_price_versions
    SET superseded_at = GREATEST(NOW(), published_at)
    WHERE product_id = v.product_id
      AND country_id IS NOT DISTINCT FROM v.country_id
      AND currency = v.currency
      AND id <> v.id
      AND superseded_at IS NULL
      AND effective_from <= v.effective_from;
    INSERT INTO recommended_prices_current (
        organization_id, brand_id, product_id, country_id, currency, version_id, price, effective_from
    )
    VALUES (v.organization_id, v.brand_id, v.product_id, v.country_id, v.currency, v.id, v.price, v.effective_from)
    ON CONFLICT ON CONSTRAINT uq_recommended_prices_current_key DO UPDATE SET
        organization_id = EXCLUDED.organization_id,
        version_id      = EXCLUDED.version_id,
        price           = EXCLUDED.price,
        effective_from  = EXCLUDED.effective_from;
END;
$$;

-- 3. Sync with product_prices.recommended_sale_price ----------------------------
-- current (country NULL) -> product_prices. A cascaded delete (product
-- removed) does not write back.
CREATE FUNCTION recommended_prices_current_sync_product_prices() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.country_id IS NULL AND pg_trigger_depth() <= 1 THEN
            UPDATE product_prices
            SET recommended_sale_price = NULL
            WHERE product_id = OLD.product_id AND currency = OLD.currency
              AND recommended_sale_price IS NOT NULL;
        END IF;
        RETURN OLD;
    END IF;
    IF NEW.country_id IS NULL THEN
        INSERT INTO product_prices (product_id, brand_id, currency, recommended_sale_price)
        VALUES (NEW.product_id, NEW.brand_id, NEW.currency, NEW.price)
        ON CONFLICT (product_id, currency) DO UPDATE SET
            recommended_sale_price = EXCLUDED.recommended_sale_price
        WHERE product_prices.recommended_sale_price IS NULL
           OR ROUND(product_prices.recommended_sale_price, 2) <> EXCLUDED.recommended_sale_price;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_recommended_prices_current_sync
    AFTER INSERT OR UPDATE OR DELETE ON recommended_prices_current
    FOR EACH ROW
    EXECUTE FUNCTION recommended_prices_current_sync_product_prices();

-- product_prices -> versions: a direct write of recommended_sale_price that
-- differs from the country-wide current price is a 'price_list' version in
-- force today; clearing it (or deleting the price row) withdraws the
-- country-wide recommendation. Values written by the sync above are equal
-- and do nothing.
CREATE FUNCTION product_prices_sync_recommended() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    cur        recommended_prices_current%ROWTYPE;
    has_cur    BOOLEAN;
    new_price  NUMERIC(18,2);
    p_product  BIGINT;
    p_currency CHAR(3);
    center_id  BIGINT;
    version_id BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.recommended_sale_price IS NULL OR pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
        new_price := NULL;
        p_product := OLD.product_id;
        p_currency := OLD.currency;
    ELSE
        new_price := ROUND(NEW.recommended_sale_price, 2);
        p_product := NEW.product_id;
        p_currency := NEW.currency;
    END IF;

    SELECT * INTO cur FROM recommended_prices_current
    WHERE product_id = p_product AND country_id IS NULL AND currency = p_currency;
    has_cur := FOUND;

    IF new_price IS NULL THEN
        IF has_cur THEN
            UPDATE recommended_price_versions
            SET superseded_at = GREATEST(NOW(), published_at)
            WHERE id = cur.version_id AND superseded_at IS NULL;
            DELETE FROM recommended_prices_current WHERE id = cur.id;
        END IF;
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    IF has_cur AND cur.price = new_price THEN
        RETURN NEW;
    END IF;

    SELECT id INTO center_id FROM organizations
    WHERE brand_id = NEW.brand_id AND type = 'center';
    IF NOT FOUND THEN
        RAISE EXCEPTION 'product_prices: brand % has no center for the recommended price version', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    UPDATE recommended_price_versions
    SET superseded_at = GREATEST(NOW(), published_at)
    WHERE product_id = NEW.product_id AND country_id IS NULL AND currency = NEW.currency
      AND effective_from = CURRENT_DATE AND superseded_at IS NULL;
    INSERT INTO recommended_price_versions (
        organization_id, brand_id, product_id, country_id, currency, price, effective_from, source
    )
    VALUES (center_id, NEW.brand_id, NEW.product_id, NULL, NEW.currency, new_price, CURRENT_DATE, 'price_list')
    RETURNING id INTO version_id;
    PERFORM recommended_prices_make_current(version_id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_product_prices_sync_recommended
    AFTER INSERT OR UPDATE OF recommended_sale_price OR DELETE ON product_prices
    FOR EACH ROW
    EXECUTE FUNCTION product_prices_sync_recommended();

-- 4. Carry over the existing recommended prices ---------------------------------
-- The version keeps the price rounded to 2 decimals; product_prices itself is
-- not rewritten (the sync compares at 2 decimals).
WITH carried AS (
    INSERT INTO recommended_price_versions (
        organization_id, brand_id, product_id, country_id, currency, price, effective_from, source,
        published_at, note
    )
    SELECT c.id, pp.brand_id, pp.product_id, NULL, pp.currency, ROUND(pp.recommended_sale_price, 2),
           CURRENT_DATE, 'migration', NOW(), ''
    FROM product_prices pp
    JOIN organizations c ON c.brand_id = pp.brand_id AND c.type = 'center'
    WHERE pp.recommended_sale_price IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM recommended_prices_current rc
                      WHERE rc.product_id = pp.product_id AND rc.country_id IS NULL
                        AND rc.currency = pp.currency)
    RETURNING id, organization_id, brand_id, product_id, country_id, currency, price, effective_from
)
INSERT INTO recommended_prices_current (
    organization_id, brand_id, product_id, country_id, currency, version_id, price, effective_from
)
SELECT organization_id, brand_id, product_id, country_id, currency, id, price, effective_from
FROM carried;

-- 5. Price discipline snapshots --------------------------------------------------
CREATE TABLE price_discipline_snapshots (
    id                      BIGSERIAL      PRIMARY KEY,
    snapshot_date           DATE           NOT NULL,
    organization_id         BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id                BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id              BIGINT         NOT NULL,
    country_id              BIGINT         NULL REFERENCES countries (id) ON DELETE RESTRICT,
    currency                CHAR(3)        NOT NULL,
    recommended_version_id  BIGINT         NULL REFERENCES recommended_price_versions (id) ON DELETE SET NULL,
    recommended_price       NUMERIC(18,2)  NOT NULL,
    list_price              NUMERIC(18,2)  NOT NULL,
    deviation_pct           NUMERIC(9,2)   NULL,
    avg_sale_price          NUMERIC(18,2)  NULL,
    sales_quantity          NUMERIC(12,2)  NOT NULL DEFAULT 0,
    computed_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_price_discipline_snapshots UNIQUE (snapshot_date, organization_id, product_id, currency),
    CONSTRAINT fk_price_discipline_snapshots_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_price_discipline_snapshots_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_price_discipline_snapshots_prices CHECK (recommended_price >= 0 AND list_price >= 0),
    CONSTRAINT chk_price_discipline_snapshots_avg CHECK (avg_sale_price IS NULL OR avg_sale_price >= 0),
    CONSTRAINT chk_price_discipline_snapshots_quantity CHECK (sales_quantity >= 0),
    -- Deviation is undefined against a zero recommendation.
    CONSTRAINT chk_price_discipline_snapshots_deviation CHECK (recommended_price > 0 OR deviation_pct IS NULL)
);

CREATE INDEX idx_price_discipline_snapshots_brand_date
    ON price_discipline_snapshots (brand_id, snapshot_date DESC);
CREATE INDEX idx_price_discipline_snapshots_org
    ON price_discipline_snapshots (organization_id, snapshot_date DESC);
CREATE INDEX idx_price_discipline_snapshots_product
    ON price_discipline_snapshots (product_id);
CREATE INDEX idx_price_discipline_snapshots_version
    ON price_discipline_snapshots (recommended_version_id)
    WHERE recommended_version_id IS NOT NULL;

-- Only organizations that sell to end customers: dealers and distributors
-- of the same brand.
CREATE FUNCTION price_discipline_snapshots_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type  TEXT;
    org_brand BIGINT;
BEGIN
    SELECT type, brand_id INTO org_type, org_brand
    FROM organizations WHERE id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_type NOT IN ('dealer', 'distributor') OR org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'price_discipline_snapshots: organization % is not a dealer or distributor of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_price_discipline_snapshots_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON price_discipline_snapshots
    FOR EACH ROW
    EXECUTE FUNCTION price_discipline_snapshots_check_org();

-- 6. Permissions. Source of truth: internal/platform/rbac/catalog.go. --------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read price discipline', 'pricing.discipline.read', 'pricing',
       ARRAY['subtree', 'brand', 'all']::text[], false, false,
       'Read dealer and distributor end-customer prices against the recommended price.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'pricing.discipline.read', 'all'),
    ('center_staff', 'pricing.discipline.read', 'brand'),
    ('distributor_owner', 'pricing.discipline.read', 'subtree')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
