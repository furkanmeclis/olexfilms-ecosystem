-- TEC-1225: receiving exceptions on stock entries.
--
-- 1. Lines carry the production date of the goods (manual entry and Excel);
--    the lot stays on the unit (units.lot).
-- 2. Attachments of an entry: waybill, invoice, photo or other document
--    (object storage key; soft delete keeps the receiving record).
-- 3. Discrepancy record ("kabul tutanağı"): one row per decision taken at
--    confirm for a missing line, an extra barcode or a damaged line.
--    Append-only: rows are written once at confirm and never changed.
-- 4. A confirmed manual entry can be undone (void movements through the
--    ledger); who, when and why are kept on the entry.

ALTER TABLE stock_entry_lines
    ADD COLUMN production_date DATE NULL;

ALTER TABLE stock_entries
    ADD COLUMN undone_at          TIMESTAMPTZ  NULL,
    ADD COLUMN undone_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN undo_reason        VARCHAR(500) NULL;

CREATE TABLE stock_entry_attachments (
    id                   BIGSERIAL     PRIMARY KEY,
    uuid                 UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT        NOT NULL,
    brand_id             BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    entry_id             BIGINT        NOT NULL REFERENCES stock_entries (id) ON DELETE CASCADE,
    kind                 VARCHAR(16)   NOT NULL,
    storage_key          VARCHAR(512)  NOT NULL,
    filename             VARCHAR(255)  NOT NULL,
    mime_type            VARCHAR(100)  NOT NULL,
    size_bytes           BIGINT        NOT NULL,
    uploaded_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ   NULL,
    CONSTRAINT uq_stock_entry_attachments_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_entry_attachments_org FOREIGN KEY (organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_stock_entry_attachments_kind CHECK (kind IN ('waybill', 'invoice', 'photo', 'other')),
    CONSTRAINT chk_stock_entry_attachments_size CHECK (size_bytes > 0)
);

CREATE INDEX idx_stock_entry_attachments_entry ON stock_entry_attachments (entry_id, id);

CREATE TABLE stock_entry_discrepancies (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    entry_id            BIGINT        NOT NULL REFERENCES stock_entries (id) ON DELETE CASCADE,
    kind                VARCHAR(16)   NOT NULL,
    decision            VARCHAR(16)   NOT NULL,
    barcode             VARCHAR(128)  NOT NULL,
    unit_id             BIGINT        NULL REFERENCES units (id) ON DELETE RESTRICT,
    product_id          BIGINT        NULL REFERENCES products (id) ON DELETE RESTRICT,
    quantity            INTEGER       NOT NULL DEFAULT 1,
    reason              VARCHAR(500)  NULL,
    -- stock_entry_attachments ids (damage photos).
    attachment_ids      BIGINT[]      NOT NULL DEFAULT '{}',
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_entry_discrepancies_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_entry_discrepancies_org FOREIGN KEY (organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_stock_entry_discrepancies_kind CHECK (kind IN ('missing', 'extra', 'damaged')),
    CONSTRAINT chk_stock_entry_discrepancies_decision CHECK (
        (kind = 'missing' AND decision = 'left_out')
        OR (kind = 'extra' AND decision IN ('added', 'rejected'))
        OR (kind = 'damaged' AND decision IN ('accepted', 'excluded'))
    ),
    -- Damage needs a reason and at least one photo.
    CONSTRAINT chk_stock_entry_discrepancies_damage CHECK (
        kind <> 'damaged' OR (reason IS NOT NULL AND btrim(reason) <> '' AND cardinality(attachment_ids) > 0)
    ),
    CONSTRAINT chk_stock_entry_discrepancies_quantity CHECK (quantity >= 1)
);

CREATE INDEX idx_stock_entry_discrepancies_entry ON stock_entry_discrepancies (entry_id, id);
CREATE INDEX idx_stock_entry_discrepancies_org_created ON stock_entry_discrepancies (organization_id, created_at);

-- Append-only: the record is written once at confirm. The organization
-- merge (TEC-1003, migration 000152) may still move rows.
CREATE FUNCTION stock_entry_discrepancies_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND current_setting('olex.org_merge', true) = 'on' THEN RETURN NEW; END IF; -- TEC-1003 org merge
    RAISE EXCEPTION 'stock_entry_discrepancies is append-only: % rejected', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_stock_entry_discrepancies_append_only
    BEFORE UPDATE ON stock_entry_discrepancies
    FOR EACH ROW
    EXECUTE FUNCTION stock_entry_discrepancies_append_only();
