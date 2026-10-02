-- TEC-184: roll split. Meters cut from a roll leave as a new unit with its
-- own barcode (same product), so a part of a roll can be assigned to an
-- order and shipped.
--
--   * stock_movements.type gains 'split'. One split writes two movements in
--     one transaction (ledger.Split): on the source roll (meters_delta = -N,
--     owner kept; status used when nothing is left) and the first movement
--     of the new unit (same owner and status, remaining = initial = N).
--   * stock_splits records the split: the source and the new unit, the
--     meters and the caller's idempotency key (one split per key and
--     organization). Both movements reference it
--     (split:stock_split:<id>:split:<barcode>).
ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_type CHECK (type IN (
    'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore',
    'order_out', 'order_cancel_restore', 'received', 'consumption', 'partial_consumption',
    'return', 'reclassification', 'count_adjustment', 'void', 'external_outbound',
    'split'
));

CREATE TABLE stock_splits (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id          BIGINT         NOT NULL,
    source_unit_id      BIGINT         NOT NULL,
    new_unit_id         BIGINT         NOT NULL,
    meters              NUMERIC(10,2)  NOT NULL,
    idempotency_key     VARCHAR(128)   NOT NULL,
    reference_type      VARCHAR(64)    NULL,
    reference_id        BIGINT         NULL,
    created_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_splits_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_splits_new_unit UNIQUE (new_unit_id),
    CONSTRAINT uq_stock_splits_org_key UNIQUE (organization_id, idempotency_key),
    CONSTRAINT fk_stock_splits_source_unit FOREIGN KEY (source_unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_splits_new_unit FOREIGN KEY (new_unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_splits_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_splits_units CHECK (source_unit_id <> new_unit_id),
    CONSTRAINT chk_stock_splits_meters CHECK (meters > 0),
    CONSTRAINT chk_stock_splits_key CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_stock_splits_reference CHECK ((reference_type IS NULL) = (reference_id IS NULL))
);

CREATE INDEX idx_stock_splits_source_unit ON stock_splits (source_unit_id);
CREATE INDEX idx_stock_splits_org_created ON stock_splits (organization_id, created_at);
