-- TEC-202 (F1-03b): barcode generation and label templates.
--
--   * label_templates: how a label is printed (code128 or QR, logo mode,
--     size and grid). Organization-scoped and brand-independent (K20): the
--     center prints unit labels for every brand it issues, the distributor
--     prints location labels for its own warehouses.
--   * barcode_counters: one sequence per brand and prefix; a batch takes a
--     contiguous range under the row lock.
--   * barcode_batches: one reservation of N barcodes for one product by the
--     center (K14). The units of a batch are created in status printed
--     (same as the import path, TEC-158) and enter stock later through
--     ledger.Post (entry, TEC-95d). units.batch_id links them.
--
-- Generated barcodes are <PREFIX>-<8 digits> (e.g. OLEX-00000123); the roll
-- split (TEC-184) appends -S<n>, so the two formats never collide.

CREATE TABLE label_templates (
    id               BIGSERIAL      PRIMARY KEY,
    uuid             UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    name             VARCHAR(100)   NOT NULL,
    kind             VARCHAR(16)    NOT NULL DEFAULT 'unit',
    symbology        VARCHAR(16)    NOT NULL DEFAULT 'code128',
    logo_mode        VARCHAR(16)    NOT NULL DEFAULT 'none',
    logo_text        VARCHAR(100)   NULL,
    -- data:image/png;base64,... (or jpeg); the only image source the
    -- document CSP allows.
    logo_image       TEXT           NULL,
    width_mm         NUMERIC(6,1)   NOT NULL DEFAULT 70,
    height_mm        NUMERIC(6,1)   NOT NULL DEFAULT 37,
    grid_columns     SMALLINT       NOT NULL DEFAULT 3,
    show_name        BOOLEAN        NOT NULL DEFAULT true,
    show_code_text   BOOLEAN        NOT NULL DEFAULT true,
    is_default       BOOLEAN        NOT NULL DEFAULT false,
    active           BOOLEAN        NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_label_templates_uuid UNIQUE (uuid),
    CONSTRAINT uq_label_templates_org_name UNIQUE (organization_id, name),
    CONSTRAINT chk_label_templates_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_label_templates_kind CHECK (kind IN ('unit', 'location')),
    CONSTRAINT chk_label_templates_symbology CHECK (symbology IN ('code128', 'qr')),
    CONSTRAINT chk_label_templates_logo_mode CHECK (logo_mode IN ('none', 'text', 'image')),
    CONSTRAINT chk_label_templates_logo CHECK (
        (logo_mode = 'none' AND logo_text IS NULL AND logo_image IS NULL)
        OR (logo_mode = 'text' AND logo_text IS NOT NULL AND logo_image IS NULL)
        OR (logo_mode = 'image' AND logo_image IS NOT NULL AND logo_text IS NULL)
    ),
    CONSTRAINT chk_label_templates_size CHECK (
        width_mm >= 20 AND width_mm <= 200 AND height_mm >= 10 AND height_mm <= 200
    ),
    CONSTRAINT chk_label_templates_columns CHECK (grid_columns >= 1 AND grid_columns <= 8)
);

CREATE INDEX idx_label_templates_org ON label_templates (organization_id, kind, name);
-- One default template per organization and kind.
CREATE UNIQUE INDEX uq_label_templates_org_default
    ON label_templates (organization_id, kind) WHERE is_default;

CREATE TRIGGER trg_label_templates_set_updated_at
    BEFORE UPDATE ON label_templates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE barcode_counters (
    brand_id   BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    prefix     VARCHAR(8)   NOT NULL,
    next_seq   BIGINT       NOT NULL DEFAULT 1,
    PRIMARY KEY (brand_id, prefix),
    CONSTRAINT chk_barcode_counters_prefix CHECK (prefix ~ '^[A-Z0-9]{2,8}$'),
    CONSTRAINT chk_barcode_counters_next CHECK (next_seq >= 1)
);

CREATE TABLE barcode_batches (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id          BIGINT         NOT NULL,
    quantity            INTEGER        NOT NULL,
    prefix              VARCHAR(8)     NOT NULL,
    first_seq           BIGINT         NOT NULL,
    last_seq            BIGINT         NOT NULL,
    meters              NUMERIC(10,2)  NULL,
    template_id         BIGINT         NULL REFERENCES label_templates (id) ON DELETE SET NULL,
    print_count         INTEGER        NOT NULL DEFAULT 0,
    last_printed_at     TIMESTAMPTZ    NULL,
    created_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_barcode_batches_uuid UNIQUE (uuid),
    CONSTRAINT fk_barcode_batches_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_barcode_batches_quantity CHECK (quantity >= 1 AND quantity <= 1000),
    CONSTRAINT chk_barcode_batches_range CHECK (first_seq >= 1 AND last_seq >= first_seq),
    CONSTRAINT chk_barcode_batches_meters CHECK (meters IS NULL OR meters > 0)
);

CREATE INDEX idx_barcode_batches_org_created ON barcode_batches (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_barcode_batches_product ON barcode_batches (product_id);

ALTER TABLE units
    ADD COLUMN batch_id BIGINT NULL REFERENCES barcode_batches (id) ON DELETE RESTRICT;

CREATE INDEX idx_units_batch ON units (batch_id) WHERE batch_id IS NOT NULL;
