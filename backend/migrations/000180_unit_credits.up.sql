-- TEC-1221 (karar: TEC-1084): kredili kod.
--
-- A serial piece unit (a barcode without meters) can be used in as many
-- services as it has credits. Rolls (meters) and fixed barcodes (pieces)
-- keep their own measure and carry no credits.
--
-- * products.usage_credits: the product default, 5.
-- * order_items.plotter_credits: a plotter cut line may override the
--   default for the units shipped on it.
-- * service_items.finished_product: "ürünü bitirdim" — the item takes all
--   the remaining credits of the unit.
-- * unit_credit_movements: append-only credit ledger; unit_credit_balances
--   is its projection. A unit gets its balance on its first credit event
--   (opening = unit_credit_default); until then its credits are the default.

ALTER TABLE products
    ADD COLUMN usage_credits INT NOT NULL DEFAULT 5,
    ADD CONSTRAINT chk_products_usage_credits CHECK (usage_credits BETWEEN 1 AND 1000);

ALTER TABLE order_items
    ADD COLUMN plotter_credits INT NULL,
    ADD CONSTRAINT chk_order_items_plotter_credits CHECK (
        plotter_credits IS NULL OR (plotter_cut AND plotter_credits BETWEEN 1 AND 1000)
    );

ALTER TABLE service_items
    ADD COLUMN finished_product BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE unit_credit_movements (
    id                BIGSERIAL     PRIMARY KEY,
    uuid              UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT        NOT NULL,
    brand_id          BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    unit_id           BIGINT        NOT NULL,
    type              VARCHAR(24)   NOT NULL,
    total_delta       INT           NOT NULL DEFAULT 0,
    remaining_delta   INT           NOT NULL DEFAULT 0,
    uses_delta        INT           NOT NULL DEFAULT 0,
    service_id        BIGINT        NULL,
    service_item_id   BIGINT        NULL,
    actor_user_id     BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    reason            TEXT          NULL,
    metadata          JSONB         NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key   VARCHAR(128)  NOT NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_unit_credit_movements_uuid UNIQUE (uuid),
    CONSTRAINT uq_unit_credit_movements_idempotency_key UNIQUE (idempotency_key),
    CONSTRAINT fk_unit_credit_movements_org FOREIGN KEY (organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_unit_credit_movements_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_unit_credit_movements_type CHECK (type IN (
        'opening', 'migration', 'service_use', 'service_finish', 'service_refund', 'increase'
    )),
    CONSTRAINT chk_unit_credit_movements_item CHECK (
        (type IN ('service_use', 'service_finish', 'service_refund'))
            = (service_id IS NOT NULL AND service_item_id IS NOT NULL)
    ),
    CONSTRAINT chk_unit_credit_movements_increase CHECK (
        type <> 'increase' OR (total_delta > 0 AND remaining_delta = total_delta AND reason IS NOT NULL)
    ),
    CONSTRAINT chk_unit_credit_movements_idempotency_key CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_unit_credit_movements_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_unit_credit_movements_unit ON unit_credit_movements (unit_id, created_at, id);
CREATE INDEX idx_unit_credit_movements_item ON unit_credit_movements (service_item_id)
    WHERE service_item_id IS NOT NULL;
CREATE INDEX idx_unit_credit_movements_org_created ON unit_credit_movements (organization_id, created_at);

CREATE FUNCTION unit_credit_movements_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND current_setting('olex.org_merge', true) = 'on' THEN RETURN NEW; END IF; -- TEC-1003 org merge
    RAISE EXCEPTION 'unit_credit_movements is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation',
              HINT = 'Correct a credit movement with a reverse movement.';
END;
$$;

CREATE TRIGGER trg_unit_credit_movements_append_only
    BEFORE UPDATE OR DELETE ON unit_credit_movements
    FOR EACH ROW
    EXECUTE FUNCTION unit_credit_movements_append_only();

CREATE TRIGGER trg_unit_credit_movements_no_truncate
    BEFORE TRUNCATE ON unit_credit_movements
    FOR EACH STATEMENT
    EXECUTE FUNCTION unit_credit_movements_append_only();

CREATE TABLE unit_credit_balances (
    unit_id            BIGINT       PRIMARY KEY,
    organization_id    BIGINT       NOT NULL,
    brand_id           BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    credits_total      INT          NOT NULL,
    credits_remaining  INT          NOT NULL,
    used_count         INT          NOT NULL DEFAULT 0,
    last_movement_id   BIGINT       NULL REFERENCES unit_credit_movements (id) ON DELETE RESTRICT,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_unit_credit_balances_org FOREIGN KEY (organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_unit_credit_balances_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_unit_credit_balances_total CHECK (credits_total >= 0),
    CONSTRAINT chk_unit_credit_balances_remaining CHECK (
        credits_remaining >= 0 AND credits_remaining <= credits_total
    ),
    CONSTRAINT chk_unit_credit_balances_used CHECK (used_count >= 0)
);

CREATE INDEX idx_unit_credit_balances_org ON unit_credit_balances (organization_id);

CREATE TRIGGER trg_unit_credit_balances_set_updated_at
    BEFORE UPDATE ON unit_credit_balances
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- unit_credit_default: the credits a unit opens with. NULL for a unit that
-- carries no credits (fixed barcode or roll). The latest plotter cut line
-- the unit was shipped on with plotter_credits wins over the product
-- default.
CREATE FUNCTION unit_credit_default(p_unit_id BIGINT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN u.unit_kind <> 'serial' OR u.initial_meters IS NOT NULL THEN NULL
        ELSE COALESCE((
            SELECT oi.plotter_credits
            FROM order_item_units oiu
            JOIN order_items oi ON oi.id = oiu.order_item_id
            WHERE oiu.unit_id = u.id AND oi.plotter_cut AND oi.plotter_credits IS NOT NULL
            ORDER BY oiu.assigned_at DESC, oiu.id DESC
            LIMIT 1
        ), p.usage_credits)
    END
    FROM units u
    JOIN products p ON p.id = u.product_id
    WHERE u.id = p_unit_id
$$;

-- unit_credits_backfill: opens the balance of every credit unit that was
-- already used in services before credits existed (this migration and the
-- legacy migrator). Uses = non-cancelled services of the unit; a consumed
-- unit (status used) has no credits left, the others keep total - uses.
-- Units that already have a balance are left alone (idempotent).
CREATE FUNCTION unit_credits_backfill() RETURNS INT
LANGUAGE plpgsql AS $$
DECLARE n INT;
BEGIN
    WITH used AS (
        SELECT u.id AS unit_id, u.organization_id, u.brand_id, u.status,
               unit_credit_default(u.id) AS total,
               COUNT(DISTINCT si.service_id)::int AS uses
        FROM units u
        JOIN service_items si ON si.unit_id = u.id
        JOIN services s ON s.id = si.service_id AND s.status <> 'cancelled'
        WHERE u.unit_kind = 'serial' AND u.initial_meters IS NULL
          AND NOT EXISTS (SELECT 1 FROM unit_credit_balances b WHERE b.unit_id = u.id)
        GROUP BY u.id
    ), calc AS (
        SELECT unit_id, organization_id, brand_id, uses,
               GREATEST(total, uses) AS total,
               CASE WHEN status = 'used' THEN 0 ELSE GREATEST(total - uses, 0) END AS remaining
        FROM used
    ), mv AS (
        INSERT INTO unit_credit_movements (
            organization_id, brand_id, unit_id, type, total_delta, remaining_delta, uses_delta,
            reason, idempotency_key
        )
        SELECT organization_id, brand_id, unit_id, 'migration', total, remaining, uses,
               'credits opened from service history', 'credit:unit:' || unit_id || ':migration'
        FROM calc
        ON CONFLICT (idempotency_key) DO NOTHING
        RETURNING id, unit_id, organization_id, brand_id, total_delta, remaining_delta, uses_delta
    )
    INSERT INTO unit_credit_balances (
        unit_id, organization_id, brand_id, credits_total, credits_remaining, used_count, last_movement_id
    )
    SELECT unit_id, organization_id, brand_id, total_delta, remaining_delta, uses_delta, id
    FROM mv;
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END;
$$;

SELECT unit_credits_backfill();
