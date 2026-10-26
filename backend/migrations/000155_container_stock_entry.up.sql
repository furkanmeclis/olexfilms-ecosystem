-- TEC-1007: stock entry from a container list (Excel). Builds on the
-- TEC-760 entry barcode import: the entry document carries the delivery
-- (supplier, container / waybill, arrival, default bin), the import reads
-- quantity / meters / lot / box / supplier code columns, the supplier's
-- product codes are remembered, column mappings are saved as templates and
-- the arrival is checked by scanning (expected vs received).

-- 1. The delivery on the entry document.
ALTER TABLE stock_entries
    ADD COLUMN supplier_id         BIGINT      NULL,
    ADD COLUMN container_no        VARCHAR(64) NULL,
    ADD COLUMN waybill_no          VARCHAR(64) NULL,
    ADD COLUMN arrival_date        DATE        NULL,
    ADD COLUMN default_location_id BIGINT      NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_stock_entries_supplier FOREIGN KEY (supplier_id, organization_id, brand_id)
        REFERENCES suppliers (id, organization_id, brand_id) ON DELETE RESTRICT DEFERRABLE INITIALLY IMMEDIATE,
    ADD CONSTRAINT chk_stock_entries_container_no CHECK (container_no IS NULL OR btrim(container_no) <> ''),
    ADD CONSTRAINT chk_stock_entries_waybill_no CHECK (waybill_no IS NULL OR btrim(waybill_no) <> '');

CREATE INDEX idx_stock_entries_container ON stock_entries (organization_id, upper(container_no))
    WHERE container_no IS NOT NULL;

-- 2. Lines: the box (carton) number of the list and the arrival scan.
ALTER TABLE stock_entry_lines
    ADD COLUMN box_no     VARCHAR(64) NULL,
    ADD COLUMN scanned_at TIMESTAMPTZ NULL,
    ADD COLUMN scanned_by BIGINT      NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_stock_entry_lines_scanned CHECK ((scanned_at IS NULL) = (scanned_by IS NULL));

-- Barcodes scanned on arrival that are not on the entry (extras).
CREATE TABLE stock_entry_scans (
    id          BIGSERIAL    PRIMARY KEY,
    entry_id    BIGINT       NOT NULL REFERENCES stock_entries (id) ON DELETE CASCADE,
    barcode     VARCHAR(64)  NOT NULL,
    unit_id     BIGINT       NULL REFERENCES units (id) ON DELETE RESTRICT,
    scanned_by  BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_stock_entry_scans_barcode CHECK (btrim(barcode) <> '')
);

CREATE UNIQUE INDEX uq_stock_entry_scans_barcode ON stock_entry_scans (entry_id, upper(barcode));

-- 3. Supplier product code -> catalog product (remembered on import).
CREATE TABLE supplier_product_codes (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL,
    brand_id        BIGINT       NOT NULL,
    supplier_id     BIGINT       NOT NULL,
    code            VARCHAR(64)  NOT NULL,
    product_id      BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_supplier_product_codes_supplier FOREIGN KEY (supplier_id, organization_id, brand_id)
        REFERENCES suppliers (id, organization_id, brand_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_supplier_product_codes_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_supplier_product_codes_code CHECK (btrim(code) <> '')
);

CREATE UNIQUE INDEX uq_supplier_product_codes ON supplier_product_codes (supplier_id, upper(code));

CREATE TRIGGER trg_supplier_product_codes_set_updated_at
    BEFORE UPDATE ON supplier_product_codes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 4. Saved column mappings of an import resource (source header -> field),
--    optionally the template of a supplier.
CREATE TABLE import_mapping_templates (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE
                                 DEFERRABLE INITIALLY IMMEDIATE,
    resource        VARCHAR(64)  NOT NULL,
    name            VARCHAR(100) NOT NULL,
    supplier_id     BIGINT       NULL REFERENCES suppliers (id) ON DELETE SET NULL,
    mapping         JSONB        NOT NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_import_mapping_templates_uuid UNIQUE (uuid),
    CONSTRAINT chk_import_mapping_templates_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_import_mapping_templates_mapping CHECK (jsonb_typeof(mapping) = 'object')
);

CREATE UNIQUE INDEX uq_import_mapping_templates_name
    ON import_mapping_templates (organization_id, resource, lower(name));

CREATE TRIGGER trg_import_mapping_templates_set_updated_at
    BEFORE UPDATE ON import_mapping_templates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
