-- TEC-1227: partial shipment with a backorder, receipt discrepancies of an
-- order and the discrepancy record (tutanak) of a warehouse transfer.
--
-- * orders.backorder_of_order_id: the order a backorder was split from. The
--   seller ships the picked part of a preparing order; the rest becomes a
--   new approved order of the same seller, buyer, brand, currency, frozen
--   rate and prices. Same seller organization and brand (composite FK to
--   uq_orders_id_org_brand); orders is rewritten by an organization merge
--   (TEC-1003), so the FK is deferred. Separate from parent_order_id
--   (supplements, TEC-1222) so a backorder is never listed as a supplement.
ALTER TABLE orders
    ADD COLUMN backorder_of_order_id BIGINT NULL,
    ADD CONSTRAINT fk_orders_backorder_of FOREIGN KEY (backorder_of_order_id, organization_id, brand_id)
        REFERENCES orders (id, organization_id, brand_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT chk_orders_backorder_not_self CHECK (backorder_of_order_id IS NULL OR backorder_of_order_id <> id);

CREATE INDEX idx_orders_backorder_of ON orders (backorder_of_order_id)
    WHERE backorder_of_order_id IS NOT NULL;

-- * order_receipt_discrepancies: what the buyer reported at receipt, one row
--   per shipped assignment (order_item_units). missing: the unit got no
--   received movement and stays in transit until resolved (delivered: the
--   buyer found it, received is posted; returned: the seller took it back,
--   order_cancel_restore is posted). damaged: the unit was received; the
--   record is closed by either side. organization_id is the order's
--   (seller) organization. No prices are stored.
CREATE TABLE order_receipt_discrepancies (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT       NOT NULL,
    brand_id             BIGINT       NOT NULL,
    order_id             BIGINT       NOT NULL,
    order_item_unit_id   BIGINT       NOT NULL REFERENCES order_item_units (id) ON DELETE RESTRICT,
    unit_id              BIGINT       NOT NULL REFERENCES units (id) ON DELETE RESTRICT,
    kind                 VARCHAR(16)  NOT NULL,
    reason               TEXT         NOT NULL,
    photo_keys           TEXT[]       NOT NULL DEFAULT '{}',
    status               VARCHAR(16)  NOT NULL DEFAULT 'open',
    resolution           VARCHAR(16)  NULL,
    resolution_note      TEXT         NULL,
    created_by_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at          TIMESTAMPTZ  NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_order_receipt_discrepancies_uuid UNIQUE (uuid),
    CONSTRAINT uq_order_receipt_discrepancies_assignment UNIQUE (order_item_unit_id),
    CONSTRAINT fk_order_receipt_discrepancies_order FOREIGN KEY (order_id, organization_id, brand_id)
        REFERENCES orders (id, organization_id, brand_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT chk_order_receipt_discrepancies_kind CHECK (kind IN ('missing', 'damaged')),
    CONSTRAINT chk_order_receipt_discrepancies_status CHECK (status IN ('open', 'resolved')),
    CONSTRAINT chk_order_receipt_discrepancies_resolution CHECK (
        (status = 'open' AND resolution IS NULL)
        OR (status = 'resolved' AND resolution IN ('delivered', 'returned', 'closed'))
    ),
    CONSTRAINT chk_order_receipt_discrepancies_photos CHECK (cardinality(photo_keys) <= 5)
);

CREATE INDEX idx_order_receipt_discrepancies_order ON order_receipt_discrepancies (order_id, id);
CREATE INDEX idx_order_receipt_discrepancies_open ON order_receipt_discrepancies (organization_id)
    WHERE status = 'open';

CREATE TRIGGER trg_order_receipt_discrepancies_set_updated_at
    BEFORE UPDATE ON order_receipt_discrepancies
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- * warehouse_transfer_discrepancies: the record of a warehouse transfer
--   completed with differences. missing: a line that did not arrive; it got
--   no transfer_in, stays in transit and keeps its unit lock (is_open) under
--   a loss investigation until resolved (found: transfer_in + placement in
--   the target warehouse; returned_to_source: transfer_cancel_restore).
--   surplus: a scanned barcode that is not on the transfer, recorded only.
CREATE TABLE warehouse_transfer_discrepancies (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    brand_id             BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    transfer_id          BIGINT       NOT NULL REFERENCES warehouse_transfers (id) ON DELETE CASCADE,
    transfer_line_id     BIGINT       NULL REFERENCES warehouse_transfer_lines (id) ON DELETE CASCADE,
    unit_id              BIGINT       NULL REFERENCES units (id) ON DELETE RESTRICT,
    barcode              TEXT         NOT NULL,
    kind                 VARCHAR(16)  NOT NULL,
    note                 TEXT         NULL,
    status               VARCHAR(16)  NOT NULL DEFAULT 'open',
    resolution           VARCHAR(24)  NULL,
    resolution_note      TEXT         NULL,
    created_by_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at          TIMESTAMPTZ  NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warehouse_transfer_discrepancies_uuid UNIQUE (uuid),
    CONSTRAINT chk_warehouse_transfer_discrepancies_kind CHECK (
        (kind = 'missing' AND transfer_line_id IS NOT NULL AND unit_id IS NOT NULL)
        OR kind = 'surplus'
    ),
    CONSTRAINT chk_warehouse_transfer_discrepancies_status CHECK (status IN ('open', 'resolved')),
    CONSTRAINT chk_warehouse_transfer_discrepancies_resolution CHECK (
        (status = 'open' AND resolution IS NULL)
        OR (status = 'resolved' AND resolution IN ('found', 'returned_to_source', 'closed'))
    )
);

CREATE UNIQUE INDEX uq_warehouse_transfer_discrepancies_line ON warehouse_transfer_discrepancies (transfer_line_id)
    WHERE transfer_line_id IS NOT NULL;
CREATE INDEX idx_warehouse_transfer_discrepancies_transfer ON warehouse_transfer_discrepancies (transfer_id, id);
CREATE INDEX idx_warehouse_transfer_discrepancies_open ON warehouse_transfer_discrepancies (organization_id)
    WHERE status = 'open';

CREATE TRIGGER trg_warehouse_transfer_discrepancies_set_updated_at
    BEFORE UPDATE ON warehouse_transfer_discrepancies
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
