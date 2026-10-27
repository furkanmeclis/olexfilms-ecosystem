-- TEC-1030: manual "invoiced" marking of received orders by the seller's
-- accounting (no integrator): one order_invoices row per external invoice
-- (number, date), its lines in order_invoice_items. orders.invoice_status
-- is the projection: pending (no line invoiced), partial, invoiced (every
-- line). Lines are the unit, so one order can carry several invoices and
-- one invoice number may span several orders.
ALTER TABLE orders ADD COLUMN invoice_status VARCHAR(16) NOT NULL DEFAULT 'pending';
ALTER TABLE orders ADD CONSTRAINT chk_orders_invoice_status
    CHECK (invoice_status IN ('pending', 'partial', 'invoiced'));
CREATE INDEX idx_orders_invoice_status
    ON orders (brand_id, organization_id, invoice_status)
    WHERE status = 'received';

CREATE TABLE order_invoices (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    order_id        BIGINT       NOT NULL,
    organization_id BIGINT       NOT NULL,
    brand_id        BIGINT       NOT NULL,
    invoice_no      VARCHAR(64)  NOT NULL,
    invoice_date    DATE         NOT NULL,
    note            TEXT         NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_order_invoices_uuid UNIQUE (uuid),
    CONSTRAINT uq_order_invoices_order_no UNIQUE (order_id, invoice_no),
    CONSTRAINT fk_order_invoices_order FOREIGN KEY (order_id, organization_id, brand_id)
        REFERENCES orders (id, organization_id, brand_id) ON DELETE CASCADE DEFERRABLE,
    CONSTRAINT chk_order_invoices_no CHECK (btrim(invoice_no) <> '')
);

CREATE INDEX idx_order_invoices_order ON order_invoices (order_id);

CREATE TABLE order_invoice_items (
    order_invoice_id BIGINT NOT NULL REFERENCES order_invoices (id) ON DELETE CASCADE,
    order_item_id    BIGINT NOT NULL REFERENCES order_items (id) ON DELETE CASCADE,
    PRIMARY KEY (order_invoice_id, order_item_id),
    CONSTRAINT uq_order_invoice_items_item UNIQUE (order_item_id)
);
