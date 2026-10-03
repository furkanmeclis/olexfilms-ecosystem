-- TEC-204 (F1-03d): stock entry documents.
--
-- A stock entry brings printed labels into stock at a center (K14):
--
--   draft -> lines (link printed barcodes / reserve new ones, TEC-202)
--         -> print labels (TEC-202 endpoints) -> place each line on a
--            location of the target warehouse -> confirmed
--
-- Confirming posts, in one transaction through ledger.Post, an entry and
-- (serial units) a placement movement per line with the idempotency keys
-- stock_entry:stock_entry_line:<line id>:<type>:<barcode>. A confirmed
-- entry is never confirmed again.
--
-- mode:
--   with_existing  lines link existing printed (or reserved) units;
--   generate_new   lines reserve a new barcode batch (TEC-202);
--   import         written by the safe stock import (TEC-158) when a batch
--                  is applied: confirmed at once, one line per applied row
--                  (import_batch_id links the batch; an undo marks the
--                  lines and, when every line is undone, the entry).
--
-- The warehouse side is brand-independent (K20), but units are looked up
-- in one brand, so the document carries the active organization's brand.

CREATE TABLE stock_entries (
    id                    BIGSERIAL     PRIMARY KEY,
    uuid                  UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    warehouse_id          BIGINT        NULL,
    mode                  VARCHAR(16)   NOT NULL,
    status                VARCHAR(16)   NOT NULL DEFAULT 'draft',
    note                  VARCHAR(500)  NULL,
    import_batch_id       BIGINT        NULL REFERENCES stock_import_batches (id) ON DELETE RESTRICT,
    created_by_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    confirmed_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    confirmed_at          TIMESTAMPTZ   NULL,
    cancelled_at          TIMESTAMPTZ   NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_entries_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_entries_import_batch UNIQUE (import_batch_id),
    CONSTRAINT fk_stock_entries_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_entries_mode CHECK (mode IN ('with_existing', 'generate_new', 'import')),
    CONSTRAINT chk_stock_entries_status CHECK (status IN ('draft', 'confirmed', 'cancelled', 'undone')),
    -- Manual entries target a warehouse; import entries come from a batch.
    CONSTRAINT chk_stock_entries_source CHECK (
        (mode = 'import' AND import_batch_id IS NOT NULL)
        OR (mode <> 'import' AND import_batch_id IS NULL AND warehouse_id IS NOT NULL)
    ),
    CONSTRAINT chk_stock_entries_confirmed CHECK ((status IN ('confirmed', 'undone')) = (confirmed_at IS NOT NULL)),
    CONSTRAINT chk_stock_entries_note CHECK (note IS NULL OR btrim(note) <> '')
);

CREATE INDEX idx_stock_entries_org_created ON stock_entries (organization_id, created_at DESC, id DESC);

CREATE TRIGGER trg_stock_entries_set_updated_at
    BEFORE UPDATE ON stock_entries
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE stock_entry_lines (
    id                     BIGSERIAL     PRIMARY KEY,
    uuid                   UUID          NOT NULL DEFAULT gen_random_uuid(),
    entry_id               BIGINT        NOT NULL REFERENCES stock_entries (id) ON DELETE CASCADE,
    unit_id                BIGINT        NOT NULL REFERENCES units (id) ON DELETE RESTRICT,
    -- Fixed barcode quantity entered (serial units: 1).
    quantity               INTEGER       NOT NULL DEFAULT 1,
    location_id            BIGINT        NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    entry_movement_id      BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    placement_movement_id  BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    undo_movement_id       BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_entry_lines_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_entry_lines_entry_unit UNIQUE (entry_id, unit_id),
    CONSTRAINT chk_stock_entry_lines_quantity CHECK (quantity >= 1)
);

CREATE INDEX idx_stock_entry_lines_entry ON stock_entry_lines (entry_id, id);
CREATE INDEX idx_stock_entry_lines_unit ON stock_entry_lines (unit_id);
