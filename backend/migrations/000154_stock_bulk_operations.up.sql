-- TEC-1008: bulk barcode operations (super_admin) and roll products.
--
-- 1. Product card: the default unit measure stays products.unit_type
--    (piece / roll_meter); a roll product carries its default roll length,
--    width, minimum cut and waste allowance.
ALTER TABLE products
    ADD COLUMN default_roll_meters NUMERIC(10,2) NULL,
    ADD COLUMN roll_width_cm       NUMERIC(6,1)  NULL,
    ADD COLUMN min_cut_meters      NUMERIC(10,2) NULL,
    ADD COLUMN waste_pct           NUMERIC(5,2)  NULL,
    ADD CONSTRAINT chk_products_roll_meters CHECK (default_roll_meters IS NULL OR default_roll_meters > 0),
    ADD CONSTRAINT chk_products_roll_width CHECK (roll_width_cm IS NULL OR roll_width_cm > 0),
    ADD CONSTRAINT chk_products_min_cut CHECK (min_cut_meters IS NULL OR min_cut_meters > 0),
    ADD CONSTRAINT chk_products_waste CHECK (waste_pct IS NULL OR (waste_pct >= 0 AND waste_pct < 100));

-- 2. Units carry an optional lot and note (set by bulk operations).
ALTER TABLE units
    ADD COLUMN lot  VARCHAR(64) NULL,
    ADD COLUMN note TEXT        NULL,
    ADD CONSTRAINT chk_units_lot CHECK (lot IS NULL OR btrim(lot) <> ''),
    ADD CONSTRAINT chk_units_note CHECK (note IS NULL OR char_length(note) <= 2000);

CREATE INDEX idx_units_lot ON units (brand_id, lot) WHERE lot IS NOT NULL;

-- 3. Mixed units in one product: a unit is a roll when it carries meters,
--    whatever the product's default measure (boxes counted as pieces are
--    converted to rolls; a product switched to roll_meter keeps its piece
--    units). The unit integrity check no longer ties meters to the product
--    type, and a partial service item needs a roll unit, not a roll product.
CREATE OR REPLACE FUNCTION units_check_integrity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type    TEXT;
    org_brand   BIGINT;
    p_fixed     BOOLEAN;
BEGIN
    IF TG_OP = 'UPDATE' AND current_setting('olex.org_merge', true) = 'on' THEN RETURN NEW; END IF; -- TEC-1003 org merge
    IF TG_OP = 'UPDATE' THEN
        IF NEW.organization_id IS DISTINCT FROM OLD.organization_id
           OR NEW.brand_id IS DISTINCT FROM OLD.brand_id
           OR NEW.barcode IS DISTINCT FROM OLD.barcode
           OR NEW.unit_kind IS DISTINCT FROM OLD.unit_kind THEN
            RAISE EXCEPTION 'unit % issuer, brand, barcode and kind are immutable', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    ELSE
        SELECT type, brand_id INTO org_type, org_brand
        FROM organizations WHERE id = NEW.organization_id;
        IF org_type IS DISTINCT FROM 'center' OR org_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'unit issuer % must be the center organization of brand %',
                NEW.organization_id, NEW.brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    SELECT uses_fixed_barcode INTO p_fixed FROM products WHERE id = NEW.product_id;
    IF p_fixed IS DISTINCT FROM (NEW.unit_kind = 'fixed') THEN
        RAISE EXCEPTION 'unit kind % does not match product % (uses_fixed_barcode=%)',
            NEW.unit_kind, NEW.product_id, p_fixed
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

DO $$
DECLARE def text;
BEGIN
    def := pg_get_functiondef('service_items_check'::regproc);
    def := replace(def,
        $r$IF NEW.kind = 'partial' AND (u_kind <> 'serial' OR p_unit_type <> 'roll_meter') THEN$r$,
        $r$IF NEW.kind = 'partial' AND (u_kind <> 'serial' OR NOT EXISTS (
        SELECT 1 FROM units ru WHERE ru.id = NEW.unit_id AND ru.initial_meters IS NOT NULL)) THEN -- TEC-1008 roll unit$r$);
    IF position('TEC-1008 roll unit' IN def) = 0 THEN
        RAISE EXCEPTION 'service_items_check: partial item rule not found';
    END IF;
    EXECUTE def;
END $$;

-- 4. Ledger: measure_conversion turns a piece unit into a roll (or back,
--    for an undo) forward only; owner, status and product stay.
ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_type CHECK (type IN (
    'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore', 'order_out', 'received',
    'order_cancel_restore', 'consumption', 'partial_consumption', 'return', 'sale', 'count_adjustment', 'void',
    'external_outbound', 'reclassification', 'split', 'measure_conversion'));

-- 5. Bulk jobs. A job is previewed (selection resolved, every unit
--    classified as eligible or excluded with its reason), then run in the
--    background after a step-up, then optionally undone. One job runs per
--    organization at a time.
CREATE TABLE stock_bulk_jobs (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    -- Deferrable like every organization key (org merges, TEC-1003).
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT
                                 DEFERRABLE INITIALLY IMMEDIATE,
    action          VARCHAR(32)  NOT NULL,
    params          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    selection       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status          VARCHAR(16)  NOT NULL DEFAULT 'preview',
    total           INTEGER      NOT NULL DEFAULT 0,
    eligible        INTEGER      NOT NULL DEFAULT 0,
    excluded        INTEGER      NOT NULL DEFAULT 0,
    succeeded       INTEGER      NOT NULL DEFAULT 0,
    failed          INTEGER      NOT NULL DEFAULT 0,
    reverted        INTEGER      NOT NULL DEFAULT 0,
    error           TEXT         NULL,
    created_by      BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    run_by          BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    undone_by       BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ  NULL,
    finished_at     TIMESTAMPTZ  NULL,
    undone_at       TIMESTAMPTZ  NULL,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_bulk_jobs_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_bulk_jobs_id_org UNIQUE (id, organization_id),
    CONSTRAINT chk_stock_bulk_jobs_action CHECK (action IN
        ('convert_to_roll', 'change_product', 'move_location', 'void', 'set_lot', 'set_note')),
    CONSTRAINT chk_stock_bulk_jobs_status CHECK (status IN
        ('preview', 'queued', 'running', 'completed', 'failed', 'undoing', 'undone', 'discarded'))
);

CREATE INDEX idx_stock_bulk_jobs_org_created ON stock_bulk_jobs (organization_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX uq_stock_bulk_jobs_active ON stock_bulk_jobs (organization_id)
    WHERE status IN ('queued', 'running', 'undoing');

CREATE TRIGGER trg_stock_bulk_jobs_set_updated_at
    BEFORE UPDATE ON stock_bulk_jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE stock_bulk_job_items (
    id              BIGSERIAL     PRIMARY KEY,
    job_id          BIGINT        NOT NULL,
    organization_id BIGINT        NOT NULL,
    unit_id         BIGINT        NOT NULL REFERENCES units (id) ON DELETE RESTRICT,
    -- eligible -> applied | failed -> reverted | revert_skipped;
    -- excluded rows never run (reason says why).
    status          VARCHAR(16)   NOT NULL,
    reason          VARCHAR(64)   NULL,
    -- Per-row override (convert_to_roll meters from the barcode list).
    meters          NUMERIC(10,2) NULL,
    previous        JSONB         NULL,
    movement_id     BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    error           TEXT          NULL,
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_stock_bulk_job_items_job FOREIGN KEY (job_id, organization_id)
        REFERENCES stock_bulk_jobs (id, organization_id) ON DELETE CASCADE DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT uq_stock_bulk_job_items_unit UNIQUE (job_id, unit_id),
    CONSTRAINT chk_stock_bulk_job_items_status CHECK (status IN
        ('eligible', 'excluded', 'applied', 'failed', 'reverted', 'revert_skipped'))
);

CREATE INDEX idx_stock_bulk_job_items_job ON stock_bulk_job_items (job_id, status, id);

-- 6. Permission (super_admin only).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Bulk barcode operations', 'stock.bulk', 'stock', ARRAY['all']::text[], true, true,
       'Convert, move, reclassify, void or annotate many barcodes at once (preview, step-up, undo).',
       m.max_sort + 10
FROM (SELECT COALESCE(MAX(sort_order), 0) AS max_sort FROM permissions) m
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r JOIN permissions p ON p.slug = 'stock.bulk'
WHERE r.slug = 'super_admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
