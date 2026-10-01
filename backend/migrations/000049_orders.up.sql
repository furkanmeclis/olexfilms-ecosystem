-- TEC-165 (F1-04a): order schema, stock reservations, sibling transfer
-- requests, order permissions. Decisions: TEC-96 orchestrator comment items
-- 1-6 and TEC-94 decision 2 (reservations live here, not in units.status).
--
--   * orders: one product order down the tree (K6: center -> distributor,
--     distributor -> dealer). organization_id is the seller and always equals
--     seller_org_id; the buyer reads its orders through buyer_org_id.
--     currency is the seller brand's brands.currency (decision 4); the rate
--     is frozen into rate_snapshot when the seller approves (decision 2), and
--     the TRY rate of that moment is kept next to it (try_rate).
--   * order_items: frozen unit price NUMERIC(14,4), line total NUMERIC(18,2)
--     (decision 6). Pieces carry quantity, roll_meter products carry meters.
--   * order_item_units: barcodes assigned to a line (and the meters cut).
--   * order_status_history: append-only transition log.
--   * stock_reservations: active|released|consumed. A serial unit (piece or
--     roll) has at most one active reservation; fixed barcodes may have many
--     (their total against the holding is checked by the use case, TEC-96c).
--   * stock_transfer_requests: dealer -> sibling dealer transfer approved by
--     the common distributor (K13).
--
-- Cancel (decision 1): before shipping the order goes straight to cancelled;
-- after shipping it passes through cancelling until the goods come back.
-- received orders are never cancelled (returns instead).
--
-- Status list: the orchestrator's draft, submitted, approved, processing,
-- shipped, received, cancelling, cancelled plus preparing, ready and
-- delivered from the warehouse state machine (TEC-96 research).

-- 1. Orders.
CREATE SEQUENCE order_no_seq;

CREATE TABLE orders (
    id                            BIGSERIAL PRIMARY KEY,
    uuid                          UUID           NOT NULL DEFAULT gen_random_uuid(),
    order_no                      VARCHAR(32)    NOT NULL
        DEFAULT ('ORD-' || lpad(nextval('order_no_seq')::text, 8, '0')),
    organization_id               BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                      BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    seller_org_id                 BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    buyer_org_id                  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    seller_warehouse_location_id  BIGINT         NULL,
    buyer_warehouse_location_id   BIGINT         NULL,
    status                        VARCHAR(16)    NOT NULL DEFAULT 'draft',
    currency                      CHAR(3)        NOT NULL,
    rate_snapshot                 JSONB          NULL,
    try_rate                      NUMERIC(20,10) NULL,
    subtotal                      NUMERIC(18,2)  NOT NULL DEFAULT 0,
    tax_total                     NUMERIC(18,2)  NOT NULL DEFAULT 0,
    total                         NUMERIC(18,2)  NOT NULL DEFAULT 0,
    delivery_mode                 VARCHAR(16)    NULL,
    tracking_no                   VARCHAR(128)   NULL,
    shipping_document_key         TEXT           NULL,
    receipt_document_key          TEXT           NULL,
    external_reference            VARCHAR(128)   NULL,
    note                          TEXT           NULL,
    cancel_reason                 TEXT           NULL,
    created_by_user_id            BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    approved_by_user_id           BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    submitted_at                  TIMESTAMPTZ    NULL,
    approved_at                   TIMESTAMPTZ    NULL,
    ready_at                      TIMESTAMPTZ    NULL,
    shipped_at                    TIMESTAMPTZ    NULL,
    delivered_at                  TIMESTAMPTZ    NULL,
    received_at                   TIMESTAMPTZ    NULL,
    cancel_requested_at           TIMESTAMPTZ    NULL,
    cancelled_at                  TIMESTAMPTZ    NULL,
    created_at                    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_orders_uuid UNIQUE (uuid),
    CONSTRAINT uq_orders_brand_order_no UNIQUE (brand_id, order_no),
    -- Target of the seller/brand-consistent FKs from order_items.
    CONSTRAINT uq_orders_id_org_brand UNIQUE (id, organization_id, brand_id),
    -- Warehouse locations belong to their side of the order.
    CONSTRAINT fk_orders_seller_location FOREIGN KEY (seller_warehouse_location_id, seller_org_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_orders_buyer_location FOREIGN KEY (buyer_warehouse_location_id, buyer_org_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_orders_org_is_seller CHECK (organization_id = seller_org_id),
    CONSTRAINT chk_orders_seller_not_buyer CHECK (seller_org_id <> buyer_org_id),
    CONSTRAINT chk_orders_order_no CHECK (btrim(order_no) <> ''),
    CONSTRAINT chk_orders_status CHECK (status IN (
        'draft', 'submitted', 'approved', 'preparing', 'ready', 'processing',
        'shipped', 'delivered', 'received', 'cancelling', 'cancelled'
    )),
    CONSTRAINT chk_orders_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_orders_rate_snapshot CHECK (rate_snapshot IS NULL OR jsonb_typeof(rate_snapshot) = 'object'),
    -- The rate is frozen exactly when the seller approves (decision 2).
    CONSTRAINT chk_orders_rate_frozen CHECK (
        (approved_at IS NULL) = (rate_snapshot IS NULL)
        AND (rate_snapshot IS NULL) = (try_rate IS NULL)
    ),
    CONSTRAINT chk_orders_try_rate CHECK (try_rate IS NULL OR try_rate > 0),
    CONSTRAINT chk_orders_totals CHECK (subtotal >= 0 AND tax_total >= 0 AND total >= 0),
    CONSTRAINT chk_orders_delivery_mode CHECK (delivery_mode IS NULL OR delivery_mode IN ('hand', 'warehouse')),
    CONSTRAINT chk_orders_external_reference CHECK (external_reference IS NULL OR btrim(external_reference) <> ''),
    CONSTRAINT chk_orders_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

CREATE INDEX idx_orders_seller_status ON orders (organization_id, status, created_at);
CREATE INDEX idx_orders_buyer_status ON orders (buyer_org_id, status, created_at);
CREATE INDEX idx_orders_brand_created ON orders (brand_id, created_at);
-- Glorian/hub reference (sync in F2).
CREATE UNIQUE INDEX uq_orders_brand_external_reference
    ON orders (brand_id, external_reference)
    WHERE external_reference IS NOT NULL;

CREATE TRIGGER trg_orders_set_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Parties: both organizations belong to the order's brand (K1/K20), the
-- buyer is a direct child of the seller and the flow is center ->
-- distributor or distributor -> dealer (K6: the center sells no product to
-- a dealer). The currency is the brand currency when it is set (decision 4).
-- Parties are checked when written; a later tree change (K25) does not
-- invalidate existing orders.
CREATE FUNCTION orders_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    s_type   TEXT;
    s_brand  BIGINT;
    b_type   TEXT;
    b_brand  BIGINT;
    b_parent BIGINT;
    b_cur    TEXT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.seller_org_id = OLD.seller_org_id
       AND NEW.buyer_org_id = OLD.buyer_org_id
       AND NEW.brand_id = OLD.brand_id
       AND NEW.currency = OLD.currency THEN
        RETURN NEW;
    END IF;
    -- chk_orders_seller_not_buyer reports this case.
    IF NEW.seller_org_id = NEW.buyer_org_id THEN
        RETURN NEW;
    END IF;

    SELECT type, brand_id INTO s_type, s_brand FROM organizations WHERE id = NEW.seller_org_id;
    SELECT type, brand_id, parent_id INTO b_type, b_brand, b_parent
    FROM organizations WHERE id = NEW.buyer_org_id;
    IF s_brand IS DISTINCT FROM NEW.brand_id OR b_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'order parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF b_parent IS DISTINCT FROM NEW.seller_org_id THEN
        RAISE EXCEPTION 'order buyer % is not a direct child of seller %',
            NEW.buyer_org_id, NEW.seller_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NOT ((s_type = 'center' AND b_type = 'distributor')
            OR (s_type = 'distributor' AND b_type = 'dealer')) THEN
        RAISE EXCEPTION 'order flow %->% is not allowed (K6)', s_type, b_type
            USING ERRCODE = 'check_violation';
    END IF;

    IF TG_OP = 'INSERT' OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.brand_id IS DISTINCT FROM OLD.brand_id THEN
        SELECT currency INTO b_cur FROM brands WHERE id = NEW.brand_id;
        IF b_cur IS DISTINCT FROM NEW.currency THEN
            RAISE EXCEPTION 'order currency % must be the brand currency %', NEW.currency, b_cur
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_orders_check_parties
    BEFORE INSERT OR UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION orders_check_parties();

-- 2. Order lines. organization_id/brand_id follow the order (composite FK).
CREATE TABLE order_items (
    id                          BIGSERIAL PRIMARY KEY,
    uuid                        UUID           NOT NULL DEFAULT gen_random_uuid(),
    order_id                    BIGINT         NOT NULL,
    organization_id             BIGINT         NOT NULL,
    brand_id                    BIGINT         NOT NULL,
    product_id                  BIGINT         NOT NULL,
    quantity                    INT            NULL,
    meters                      NUMERIC(10,2)  NULL,
    unit_price                  NUMERIC(14,4)  NOT NULL,
    price_source                VARCHAR(32)    NOT NULL,
    recommended_price_snapshot  NUMERIC(14,4)  NULL,
    line_total                  NUMERIC(18,2)  NOT NULL,
    note                        TEXT           NULL,
    created_at                  TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_order_items_uuid UNIQUE (uuid),
    CONSTRAINT uq_order_items_order_product UNIQUE (order_id, product_id),
    CONSTRAINT uq_order_items_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_order_items_order FOREIGN KEY (order_id, organization_id, brand_id)
        REFERENCES orders (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_order_items_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_order_items_amount CHECK (
        (quantity IS NULL) <> (meters IS NULL)
        AND (quantity IS NULL OR quantity > 0)
        AND (meters IS NULL OR meters > 0)
    ),
    CONSTRAINT chk_order_items_unit_price CHECK (unit_price >= 0),
    CONSTRAINT chk_order_items_recommended CHECK (recommended_price_snapshot IS NULL OR recommended_price_snapshot >= 0),
    CONSTRAINT chk_order_items_line_total CHECK (line_total >= 0),
    CONSTRAINT chk_order_items_price_source CHECK (price_source IN ('list', 'override', 'distributor_dealer'))
);

CREATE INDEX idx_order_items_product ON order_items (product_id);

CREATE TRIGGER trg_order_items_set_updated_at
    BEFORE UPDATE ON order_items
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- roll_meter products are ordered in meters, pieces in quantity.
CREATE FUNCTION order_items_check_amount() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    p_unit_type TEXT;
BEGIN
    SELECT unit_type INTO p_unit_type FROM products WHERE id = NEW.product_id;
    IF (p_unit_type = 'roll_meter') IS DISTINCT FROM (NEW.meters IS NOT NULL) THEN
        RAISE EXCEPTION 'order line of product % must use %', NEW.product_id,
            CASE WHEN p_unit_type = 'roll_meter' THEN 'meters' ELSE 'quantity' END
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_order_items_check_amount
    BEFORE INSERT OR UPDATE OF product_id, quantity, meters ON order_items
    FOR EACH ROW
    EXECUTE FUNCTION order_items_check_amount();

-- 3. Stock reservations (TEC-94 decision 2). unit_kind is pinned to the
-- unit through the FK so the partial unique index can tell serial units
-- (one active reservation) from fixed barcodes (many).
CREATE TABLE stock_reservations (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT         NOT NULL,
    brand_id         BIGINT         NOT NULL,
    unit_id          BIGINT         NOT NULL,
    unit_kind        VARCHAR(16)    NOT NULL,
    order_item_id    BIGINT         NOT NULL,
    quantity         INT            NULL,
    meters           NUMERIC(10,2)  NULL,
    status           VARCHAR(16)    NOT NULL DEFAULT 'active',
    released_at      TIMESTAMPTZ    NULL,
    consumed_at      TIMESTAMPTZ    NULL,
    created_by_user_id BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_reservations_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_reservations_unit FOREIGN KEY (unit_id, brand_id, unit_kind)
        REFERENCES units (id, brand_id, unit_kind) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_reservations_item FOREIGN KEY (order_item_id, organization_id, brand_id)
        REFERENCES order_items (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_reservations_amount CHECK (
        (quantity IS NULL) <> (meters IS NULL)
        AND (quantity IS NULL OR quantity > 0)
        AND (meters IS NULL OR meters > 0)
    ),
    CONSTRAINT chk_stock_reservations_status CHECK (status IN ('active', 'released', 'consumed')),
    CONSTRAINT chk_stock_reservations_released CHECK ((status = 'released') = (released_at IS NOT NULL)),
    CONSTRAINT chk_stock_reservations_consumed CHECK ((status = 'consumed') = (consumed_at IS NOT NULL))
);

-- A serial unit (piece or roll) is reserved by one order line at a time.
CREATE UNIQUE INDEX uq_stock_reservations_active_serial
    ON stock_reservations (unit_id)
    WHERE status = 'active' AND unit_kind = 'serial';
-- One active reservation per (line, unit) for fixed barcodes as well.
CREATE UNIQUE INDEX uq_stock_reservations_active_item_unit
    ON stock_reservations (order_item_id, unit_id)
    WHERE status = 'active';
CREATE INDEX idx_stock_reservations_unit_status ON stock_reservations (unit_id, status);
CREATE INDEX idx_stock_reservations_org_status ON stock_reservations (organization_id, status, created_at);

CREATE TRIGGER trg_stock_reservations_set_updated_at
    BEFORE UPDATE ON stock_reservations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 4. Units assigned to an order line (barcode assignment / scan).
CREATE TABLE order_item_units (
    id                   BIGSERIAL PRIMARY KEY,
    order_item_id        BIGINT         NOT NULL,
    organization_id      BIGINT         NOT NULL,
    brand_id             BIGINT         NOT NULL,
    unit_id              BIGINT         NOT NULL,
    quantity             INT            NULL,
    meters               NUMERIC(10,2)  NULL,
    reservation_id       BIGINT         NULL REFERENCES stock_reservations (id) ON DELETE RESTRICT,
    movement_id          BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    assigned_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    assigned_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_order_item_units_item_unit UNIQUE (order_item_id, unit_id),
    CONSTRAINT fk_order_item_units_item FOREIGN KEY (order_item_id, organization_id, brand_id)
        REFERENCES order_items (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_order_item_units_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_order_item_units_amount CHECK (
        (quantity IS NULL) <> (meters IS NULL)
        AND (quantity IS NULL OR quantity > 0)
        AND (meters IS NULL OR meters > 0)
    )
);

CREATE INDEX idx_order_item_units_unit ON order_item_units (unit_id);

CREATE TRIGGER trg_order_item_units_set_updated_at
    BEFORE UPDATE ON order_item_units
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- A reserved or assigned unit is of the line's product and uses the line's
-- measure (meters for roll_meter lines, quantity otherwise). Shared by
-- stock_reservations and order_item_units (same column names).
CREATE FUNCTION order_unit_check_item() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    u_product BIGINT;
    i_product BIGINT;
    i_meters  NUMERIC;
BEGIN
    SELECT product_id INTO u_product FROM units WHERE id = NEW.unit_id;
    SELECT product_id, meters INTO i_product, i_meters FROM order_items WHERE id = NEW.order_item_id;
    IF u_product IS DISTINCT FROM i_product THEN
        RAISE EXCEPTION 'unit % is not of the product of order line %', NEW.unit_id, NEW.order_item_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF (i_meters IS NOT NULL) IS DISTINCT FROM (NEW.meters IS NOT NULL) THEN
        RAISE EXCEPTION 'unit % on order line % must use the line measure', NEW.unit_id, NEW.order_item_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_stock_reservations_check_item
    BEFORE INSERT OR UPDATE OF unit_id, order_item_id, quantity, meters ON stock_reservations
    FOR EACH ROW
    EXECUTE FUNCTION order_unit_check_item();

CREATE TRIGGER trg_order_item_units_check_item
    BEFORE INSERT OR UPDATE OF unit_id, order_item_id, quantity, meters ON order_item_units
    FOR EACH ROW
    EXECUTE FUNCTION order_unit_check_item();

-- 5. Status history (append-only).
CREATE TABLE order_status_history (
    id               BIGSERIAL PRIMARY KEY,
    order_id         BIGINT        NOT NULL,
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    from_status      VARCHAR(16)   NULL,
    to_status        VARCHAR(16)   NOT NULL,
    actor_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    reason           TEXT          NULL,
    metadata         JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_order_status_history_order FOREIGN KEY (order_id, organization_id, brand_id)
        REFERENCES orders (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_order_status_history_from CHECK (from_status IS NULL OR from_status IN (
        'draft', 'submitted', 'approved', 'preparing', 'ready', 'processing',
        'shipped', 'delivered', 'received', 'cancelling', 'cancelled'
    )),
    CONSTRAINT chk_order_status_history_to CHECK (to_status IN (
        'draft', 'submitted', 'approved', 'preparing', 'ready', 'processing',
        'shipped', 'delivered', 'received', 'cancelling', 'cancelled'
    )),
    CONSTRAINT chk_order_status_history_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_order_status_history_order ON order_status_history (order_id, created_at, id);
CREATE INDEX idx_order_status_history_org_created ON order_status_history (organization_id, created_at);

CREATE FUNCTION order_status_history_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'order_status_history is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_order_status_history_append_only
    BEFORE UPDATE OR DELETE ON order_status_history
    FOR EACH ROW
    EXECUTE FUNCTION order_status_history_append_only();

CREATE TRIGGER trg_order_status_history_no_truncate
    BEFORE TRUNCATE ON order_status_history
    FOR EACH STATEMENT
    EXECUTE FUNCTION order_status_history_append_only();

-- 6. Dealer -> sibling dealer transfer requests (K13). The giving dealer A
-- (from_org_id = organization_id) requests, the common distributor approves,
-- the price is A's purchase price (frozen on approval); A is credited and
-- B charged (TEC-96f / TEC-170).
CREATE TABLE stock_transfer_requests (
    id                    BIGSERIAL PRIMARY KEY,
    uuid                  UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    from_org_id           BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    to_org_id             BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    approver_org_id       BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    product_id            BIGINT         NOT NULL,
    unit_id               BIGINT         NULL,
    quantity              INT            NULL,
    meters                NUMERIC(10,2)  NULL,
    currency              CHAR(3)        NOT NULL,
    unit_price            NUMERIC(14,4)  NULL,
    line_total            NUMERIC(18,2)  NULL,
    rate_snapshot         JSONB          NULL,
    status                VARCHAR(16)    NOT NULL DEFAULT 'requested',
    reason                TEXT           NULL,
    requested_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_by_user_id    BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at            TIMESTAMPTZ    NULL,
    decision_note         TEXT           NULL,
    completed_at          TIMESTAMPTZ    NULL,
    cancelled_at          TIMESTAMPTZ    NULL,
    created_at            TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_transfer_requests_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_transfer_requests_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_transfer_requests_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_transfer_requests_org_is_from CHECK (organization_id = from_org_id),
    CONSTRAINT chk_stock_transfer_requests_parties CHECK (
        from_org_id <> to_org_id AND approver_org_id <> from_org_id AND approver_org_id <> to_org_id
    ),
    CONSTRAINT chk_stock_transfer_requests_amount CHECK (
        (quantity IS NULL) <> (meters IS NULL)
        AND (quantity IS NULL OR quantity > 0)
        AND (meters IS NULL OR meters > 0)
    ),
    CONSTRAINT chk_stock_transfer_requests_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_stock_transfer_requests_prices CHECK (
        (unit_price IS NULL OR unit_price >= 0) AND (line_total IS NULL OR line_total >= 0)
    ),
    CONSTRAINT chk_stock_transfer_requests_rate_snapshot CHECK (
        rate_snapshot IS NULL OR jsonb_typeof(rate_snapshot) = 'object'
    ),
    CONSTRAINT chk_stock_transfer_requests_status CHECK (status IN (
        'requested', 'approved', 'rejected', 'completed', 'cancelled'
    )),
    CONSTRAINT chk_stock_transfer_requests_decided CHECK (
        (status IN ('approved', 'rejected', 'completed')) = (decided_at IS NOT NULL)
        OR (status = 'cancelled')
    ),
    -- The price is frozen when the distributor approves.
    CONSTRAINT chk_stock_transfer_requests_priced CHECK (
        status NOT IN ('approved', 'completed') OR (unit_price IS NOT NULL AND line_total IS NOT NULL)
    ),
    CONSTRAINT chk_stock_transfer_requests_completed CHECK ((status = 'completed') = (completed_at IS NOT NULL)),
    CONSTRAINT chk_stock_transfer_requests_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

CREATE INDEX idx_stock_transfer_requests_from ON stock_transfer_requests (organization_id, status, created_at);
CREATE INDEX idx_stock_transfer_requests_to ON stock_transfer_requests (to_org_id, status, created_at);
CREATE INDEX idx_stock_transfer_requests_approver ON stock_transfer_requests (approver_org_id, status, created_at);

CREATE TRIGGER trg_stock_transfer_requests_set_updated_at
    BEFORE UPDATE ON stock_transfer_requests
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Both sides are dealers of the brand under the approving distributor (K13).
CREATE FUNCTION stock_transfer_requests_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    f_type TEXT; f_brand BIGINT; f_parent BIGINT;
    t_type TEXT; t_brand BIGINT; t_parent BIGINT;
    a_type TEXT; a_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.from_org_id = OLD.from_org_id
       AND NEW.to_org_id = OLD.to_org_id
       AND NEW.approver_org_id = OLD.approver_org_id
       AND NEW.brand_id = OLD.brand_id THEN
        RETURN NEW;
    END IF;
    SELECT type, brand_id, parent_id INTO f_type, f_brand, f_parent FROM organizations WHERE id = NEW.from_org_id;
    SELECT type, brand_id, parent_id INTO t_type, t_brand, t_parent FROM organizations WHERE id = NEW.to_org_id;
    SELECT type, brand_id INTO a_type, a_brand FROM organizations WHERE id = NEW.approver_org_id;
    IF f_brand IS DISTINCT FROM NEW.brand_id OR t_brand IS DISTINCT FROM NEW.brand_id
       OR a_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'transfer parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF f_type IS DISTINCT FROM 'dealer' OR t_type IS DISTINCT FROM 'dealer'
       OR a_type IS DISTINCT FROM 'distributor'
       OR f_parent IS DISTINCT FROM NEW.approver_org_id
       OR t_parent IS DISTINCT FROM NEW.approver_org_id THEN
        RAISE EXCEPTION 'transfer must be between sibling dealers of approver % (K13)', NEW.approver_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_stock_transfer_requests_check_parties
    BEFORE INSERT OR UPDATE ON stock_transfer_requests
    FOR EACH ROW
    EXECUTE FUNCTION stock_transfer_requests_check_parties();

-- 7. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (TestMigrationMatchesCatalog compares the two).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read orders', 'orders.read', 'orders',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Orders the organization sells or buys.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write orders', 'orders.write', 'orders',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Create and edit draft orders and submit them to the supplier.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Approve orders', 'orders.approve', 'orders',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Approve incoming orders as the seller; freezes prices and the exchange rate (K7).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Ship orders', 'orders.ship', 'orders',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Assign barcodes, prepare, ship and deliver orders as the seller.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Receive orders', 'orders.receive', 'orders',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Confirm receipt of delivered orders as the buyer.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Cancel orders', 'orders.cancel', 'orders',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Cancel orders; after shipping the order waits in cancelling until the goods return.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Request stock transfers', 'transfers.request', 'transfers',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Request a stock transfer to a sibling dealer (K13).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Approve stock transfers', 'transfers.approve', 'transfers',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Approve or reject transfers between dealers of the distributor (K13).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'orders.read', 'all'),
    ('super_admin', 'orders.write', 'all'),
    ('super_admin', 'orders.approve', 'all'),
    ('super_admin', 'orders.ship', 'all'),
    ('super_admin', 'orders.receive', 'all'),
    ('super_admin', 'orders.cancel', 'all'),
    ('super_admin', 'transfers.request', 'all'),
    ('super_admin', 'transfers.approve', 'all'),
    ('center_staff', 'orders.read', 'brand'),
    ('center_staff', 'orders.write', 'brand'),
    ('center_staff', 'orders.approve', 'brand'),
    ('center_staff', 'orders.cancel', 'brand'),
    ('center_warehouse', 'orders.read', 'brand'),
    ('center_warehouse', 'orders.ship', 'brand'),
    ('center_accounting', 'orders.read', 'brand'),
    ('distributor_owner', 'orders.read', 'managed'),
    ('distributor_owner', 'orders.write', 'managed'),
    ('distributor_owner', 'orders.approve', 'managed'),
    ('distributor_owner', 'orders.ship', 'managed'),
    ('distributor_owner', 'orders.receive', 'managed'),
    ('distributor_owner', 'orders.cancel', 'managed'),
    ('distributor_owner', 'transfers.approve', 'managed'),
    ('distributor_warehouse_staff', 'orders.read', 'managed'),
    ('distributor_warehouse_staff', 'orders.ship', 'managed'),
    ('distributor_warehouse_staff', 'orders.receive', 'managed'),
    ('distributor_accounting', 'orders.read', 'managed'),
    ('dealer_owner', 'orders.read', 'managed'),
    ('dealer_owner', 'orders.write', 'managed'),
    ('dealer_owner', 'orders.receive', 'managed'),
    ('dealer_owner', 'orders.cancel', 'managed'),
    ('dealer_owner', 'transfers.request', 'managed'),
    ('dealer_staff', 'orders.read', 'managed'),
    ('dealer_staff', 'orders.receive', 'managed'),
    ('dealer_accounting', 'orders.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
