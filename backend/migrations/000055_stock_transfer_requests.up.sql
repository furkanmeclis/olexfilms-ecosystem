-- TEC-197 (F1-04f2): stock transfer requests between sibling dealers or
-- sibling distributors (same parent, same brand, K1/K13/K20).
--
-- stock_transfer_requests (000049, no reader or writer yet) becomes the
-- request header and gets one row per unit in stock_transfer_request_items:
--
--   * header: the giving organization (from_org_id = organization_id) asks
--     to hand units over to a sibling (to_org_id); approver_org_id is the
--     common parent. Status flow:
--       requested -> approved | rejected   (receiver or common parent, K13)
--       approved  -> shipped               (giver; transfer_out per unit)
--       shipped   -> received              (receiver; transfer_in per unit)
--       requested | approved -> cancelled  (no stock movement)
--       shipped   -> cancelled             (giver; transfer_cancel_restore)
--     A rejected request never writes a stock movement. The single-line
--     columns of 000049 (product_id, unit_id, quantity, meters, unit_price,
--     line_total, rate_snapshot, completed_at) stay NULL on new rows: the
--     units and their prices live in the items.
--   * items: the unit (serial piece/roll, or a fixed barcode with a
--     quantity), the giver's purchase price frozen at approval (K13: the
--     transfer price is A's purchase price) and the ledger movements.
--
-- Accounting (K13: A credited / B charged) is not booked here; the frozen
-- prices keep the data for it.

-- 1. Header.
CREATE SEQUENCE stock_transfer_no_seq;

ALTER TABLE stock_transfer_requests
    ALTER COLUMN product_id DROP NOT NULL,
    ADD COLUMN transfer_no VARCHAR(32) NOT NULL
        DEFAULT ('TRF-' || lpad(nextval('stock_transfer_no_seq')::text, 8, '0')),
    ADD COLUMN total                NUMERIC(18,2) NULL,
    ADD COLUMN cancel_reason        TEXT          NULL,
    ADD COLUMN shipped_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN received_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN cancelled_by_user_id BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN shipped_at           TIMESTAMPTZ   NULL,
    ADD COLUMN received_at          TIMESTAMPTZ   NULL,
    DROP CONSTRAINT chk_stock_transfer_requests_amount,
    DROP CONSTRAINT chk_stock_transfer_requests_priced,
    DROP CONSTRAINT chk_stock_transfer_requests_decided,
    DROP CONSTRAINT chk_stock_transfer_requests_completed,
    DROP CONSTRAINT chk_stock_transfer_requests_status;

ALTER TABLE stock_transfer_requests
    ADD CONSTRAINT uq_stock_transfer_requests_brand_no UNIQUE (brand_id, transfer_no),
    ADD CONSTRAINT uq_stock_transfer_requests_id_org_brand UNIQUE (id, organization_id, brand_id),
    ADD CONSTRAINT chk_stock_transfer_requests_transfer_no CHECK (btrim(transfer_no) <> ''),
    ADD CONSTRAINT chk_stock_transfer_requests_status CHECK (status IN (
        'requested', 'approved', 'rejected', 'shipped', 'received', 'cancelled'
    )),
    ADD CONSTRAINT chk_stock_transfer_requests_total CHECK (total IS NULL OR total >= 0),
    ADD CONSTRAINT chk_stock_transfer_requests_decided CHECK (
        status NOT IN ('approved', 'rejected', 'shipped', 'received') OR decided_at IS NOT NULL
    ),
    ADD CONSTRAINT chk_stock_transfer_requests_shipped CHECK (
        status NOT IN ('shipped', 'received') OR shipped_at IS NOT NULL
    ),
    ADD CONSTRAINT chk_stock_transfer_requests_received CHECK ((status = 'received') = (received_at IS NOT NULL));

-- Both sides are of the request's brand, of the same type (dealer or
-- distributor) and direct children of approver_org_id, the common parent
-- (K13: no transfer across parents or brands). Checked when written; a later
-- tree change (K25) does not invalidate existing requests.
CREATE OR REPLACE FUNCTION stock_transfer_requests_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    f_type TEXT; f_brand BIGINT; f_parent BIGINT;
    t_type TEXT; t_brand BIGINT; t_parent BIGINT;
    p_brand BIGINT;
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
    SELECT brand_id INTO p_brand FROM organizations WHERE id = NEW.approver_org_id;
    IF f_brand IS DISTINCT FROM NEW.brand_id OR t_brand IS DISTINCT FROM NEW.brand_id
       OR p_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'transfer parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF f_type IS DISTINCT FROM t_type OR f_type NOT IN ('dealer', 'distributor')
       OR f_parent IS DISTINCT FROM NEW.approver_org_id
       OR t_parent IS DISTINCT FROM NEW.approver_org_id THEN
        RAISE EXCEPTION 'transfer must be between siblings of parent % (K13)', NEW.approver_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

-- 2. One row per unit. organization_id/brand_id follow the request
-- (composite FK); quantity is set for fixed barcodes only, meters snapshot
-- a roll's remaining length.
CREATE TABLE stock_transfer_request_items (
    id                   BIGSERIAL PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    request_id           BIGINT         NOT NULL,
    organization_id      BIGINT         NOT NULL,
    brand_id             BIGINT         NOT NULL,
    unit_id              BIGINT         NOT NULL,
    product_id           BIGINT         NOT NULL,
    quantity             INT            NULL,
    meters               NUMERIC(10,2)  NULL,
    unit_price           NUMERIC(14,4)  NULL,
    line_total           NUMERIC(18,2)  NULL,
    out_movement_id      BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    in_movement_id       BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    restore_movement_id  BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_transfer_request_items_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_transfer_request_items_unit UNIQUE (request_id, unit_id),
    CONSTRAINT fk_stock_transfer_request_items_request FOREIGN KEY (request_id, organization_id, brand_id)
        REFERENCES stock_transfer_requests (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_transfer_request_items_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_transfer_request_items_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_transfer_request_items_quantity CHECK (quantity IS NULL OR quantity > 0),
    CONSTRAINT chk_stock_transfer_request_items_meters CHECK (meters IS NULL OR meters > 0),
    CONSTRAINT chk_stock_transfer_request_items_prices CHECK (
        (unit_price IS NULL OR unit_price >= 0) AND (line_total IS NULL OR line_total >= 0)
    )
);

CREATE INDEX idx_stock_transfer_request_items_unit ON stock_transfer_request_items (unit_id);

CREATE TRIGGER trg_stock_transfer_request_items_set_updated_at
    BEFORE UPDATE ON stock_transfer_request_items
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 3. Permissions: sibling distributors request transfers too. Source of
-- truth: internal/platform/rbac/catalog.go (TestMigrationMatchesCatalog).
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r
JOIN permissions p ON p.slug = 'transfers.request'
WHERE r.slug = 'distributor_owner'
ON CONFLICT DO NOTHING;
