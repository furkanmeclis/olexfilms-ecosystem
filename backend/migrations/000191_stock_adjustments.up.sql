-- TEC-1226: stock adjustment documents and return inspection.
--
-- An adjustment document (waste, damage, scrap, loss, found, other) holds
-- lines of units held by the organization: a serial unit as a whole, a
-- part of a roll in meters or a quantity of a fixed barcode. Flow:
-- draft -> pending_approval -> posted (or cancelled). Posting writes the
-- ledger: a whole serial unit is voided (found: brought back with the
-- `found` movement), roll meters and fixed quantities are a count_adjustment.
-- Above the organization's threshold (pieces and meters separately) a second
-- person approves; the creator never approves such a document.
--
-- A returned serial unit enters quarantine when its return is received
-- (ledger status quarantine, 000184). The receiving organization inspects it:
-- back to stock (optionally into a bin), scrap (an adjustment document) or
-- back to the supplier (external_outbound). Each decision is one
-- return_inspections row.

CREATE SEQUENCE stock_adjustment_no_seq;

CREATE TABLE stock_adjustment_settings (
    -- Deferrable like every organization key (org merges, TEC-1003).
    organization_id     BIGINT        PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE
                                      DEFERRABLE INITIALLY IMMEDIATE,
    threshold_quantity  INT           NOT NULL DEFAULT 5,
    threshold_meters    NUMERIC(10,2) NOT NULL DEFAULT 25,
    updated_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_stock_adjustment_settings_quantity CHECK (threshold_quantity >= 0),
    CONSTRAINT chk_stock_adjustment_settings_meters CHECK (threshold_meters >= 0)
);

CREATE TABLE stock_adjustments (
    id                        BIGSERIAL     PRIMARY KEY,
    uuid                      UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id           BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT
                                            DEFERRABLE INITIALLY IMMEDIATE,
    brand_id                  BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    adjustment_no             VARCHAR(32)   NOT NULL
                                            DEFAULT ('ADJ-' || lpad(nextval('stock_adjustment_no_seq')::text, 8, '0')),
    warehouse_id              BIGINT        NULL,
    type                      VARCHAR(16)   NOT NULL,
    reason_code               VARCHAR(64)   NULL,
    note                      VARCHAR(1000) NULL,
    source                    VARCHAR(24)   NOT NULL DEFAULT 'manual',
    status                    VARCHAR(20)   NOT NULL DEFAULT 'draft',
    -- Totals of the lines, frozen at submit (the threshold check).
    total_quantity            INT           NOT NULL DEFAULT 0,
    total_meters              NUMERIC(12,2) NOT NULL DEFAULT 0,
    requires_second_approval  BOOLEAN       NOT NULL DEFAULT FALSE,
    created_by_user_id        BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    submitted_by_user_id      BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    submitted_at              TIMESTAMPTZ   NULL,
    approved_by_user_id       BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    posted_at                 TIMESTAMPTZ   NULL,
    cancelled_by_user_id      BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    cancelled_at              TIMESTAMPTZ   NULL,
    cancel_reason             VARCHAR(500)  NULL,
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_adjustments_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_adjustments_no UNIQUE (adjustment_no),
    CONSTRAINT uq_stock_adjustments_id_org UNIQUE (id, organization_id),
    CONSTRAINT fk_stock_adjustments_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_stock_adjustments_type CHECK (type IN ('waste', 'damage', 'scrap', 'loss', 'found', 'other')),
    CONSTRAINT chk_stock_adjustments_source CHECK (source IN ('manual', 'return_inspection')),
    CONSTRAINT chk_stock_adjustments_status CHECK (status IN ('draft', 'pending_approval', 'posted', 'cancelled')),
    CONSTRAINT chk_stock_adjustments_posted CHECK ((status = 'posted') = (posted_at IS NOT NULL)),
    CONSTRAINT chk_stock_adjustments_note CHECK (note IS NULL OR btrim(note) <> ''),
    CONSTRAINT chk_stock_adjustments_reason CHECK (reason_code IS NULL OR btrim(reason_code) <> '')
);

CREATE INDEX idx_stock_adjustments_org ON stock_adjustments (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_stock_adjustments_org_status ON stock_adjustments (organization_id, status);

CREATE TABLE stock_adjustment_lines (
    id                BIGSERIAL     PRIMARY KEY,
    uuid              UUID          NOT NULL DEFAULT gen_random_uuid(),
    adjustment_id     BIGINT        NOT NULL,
    organization_id   BIGINT        NOT NULL,
    unit_id           BIGINT        NOT NULL REFERENCES units (id) ON DELETE RESTRICT,
    product_id        BIGINT        NOT NULL REFERENCES products (id) ON DELETE RESTRICT,
    -- whole: a serial unit as a whole; meters: part of a roll; quantity: a
    -- fixed barcode quantity.
    kind              VARCHAR(16)   NOT NULL,
    quantity          INT           NULL,
    meters            NUMERIC(10,2) NULL,
    -- Remaining meters of a whole roll when the line was added (threshold).
    roll_meters       NUMERIC(10,2) NULL,
    -- The bin a found unit goes to / a fixed quantity is taken from.
    location_id       BIGINT        NULL,
    movement_id       BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_adjustment_lines_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_adjustment_lines_unit UNIQUE (adjustment_id, unit_id),
    CONSTRAINT fk_stock_adjustment_lines_adjustment FOREIGN KEY (adjustment_id, organization_id)
        REFERENCES stock_adjustments (id, organization_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_stock_adjustment_lines_location FOREIGN KEY (location_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_stock_adjustment_lines_kind CHECK (
        (kind = 'whole' AND quantity IS NULL AND meters IS NULL)
        OR (kind = 'meters' AND quantity IS NULL AND meters > 0)
        OR (kind = 'quantity' AND quantity > 0 AND meters IS NULL))
);

CREATE INDEX idx_stock_adjustment_lines_unit ON stock_adjustment_lines (unit_id);

CREATE TABLE stock_adjustment_photos (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    adjustment_id       BIGINT       NOT NULL,
    organization_id     BIGINT       NOT NULL,
    object_key          TEXT         NOT NULL,
    content_type        VARCHAR(64)  NOT NULL,
    size_bytes          BIGINT       NOT NULL,
    uploaded_by_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_adjustment_photos_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_adjustment_photos_adjustment FOREIGN KEY (adjustment_id, organization_id)
        REFERENCES stock_adjustments (id, organization_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_stock_adjustment_photos_size CHECK (size_bytes > 0)
);

CREATE INDEX idx_stock_adjustment_photos_adjustment ON stock_adjustment_photos (adjustment_id);

CREATE TABLE return_inspections (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT
                                     DEFERRABLE INITIALLY IMMEDIATE,
    brand_id            BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    unit_id             BIGINT       NOT NULL REFERENCES units (id) ON DELETE RESTRICT,
    -- The return line the unit came back on (NULL: quarantined otherwise).
    transfer_item_id    BIGINT       NULL REFERENCES stock_transfer_request_items (id) ON DELETE RESTRICT,
    decision            VARCHAR(16)  NOT NULL,
    return_reason       VARCHAR(32)  NOT NULL,
    note                VARCHAR(1000) NULL,
    location_id         BIGINT       NULL,
    adjustment_id       BIGINT       NULL,
    movement_id         BIGINT       NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    decided_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_return_inspections_uuid UNIQUE (uuid),
    CONSTRAINT fk_return_inspections_location FOREIGN KEY (location_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_return_inspections_adjustment FOREIGN KEY (adjustment_id, organization_id)
        REFERENCES stock_adjustments (id, organization_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_return_inspections_decision CHECK (decision IN ('restock', 'scrap', 'supplier_return')),
    CONSTRAINT chk_return_inspections_reason CHECK (return_reason IN (
        'defective', 'damaged', 'wrong_product', 'excess', 'customer_return', 'other')),
    CONSTRAINT chk_return_inspections_scrap CHECK ((decision = 'scrap') = (adjustment_id IS NOT NULL)),
    CONSTRAINT chk_return_inspections_note CHECK (note IS NULL OR btrim(note) <> '')
);

CREATE INDEX idx_return_inspections_org ON return_inspections (organization_id, created_at DESC);
CREATE INDEX idx_return_inspections_unit ON return_inspections (unit_id);
