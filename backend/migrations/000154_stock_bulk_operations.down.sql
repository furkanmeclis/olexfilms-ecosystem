DELETE FROM permissions WHERE slug = 'stock.bulk';

DROP TABLE IF EXISTS stock_bulk_job_items;
DROP TABLE IF EXISTS stock_bulk_jobs;

-- Conversions written by bulk jobs go with the feature (append-only ledger:
-- the trigger is bypassed for this one statement, like 000135's down).
ALTER TABLE stock_movements DISABLE TRIGGER trg_stock_movements_append_only;
DELETE FROM stock_movements WHERE type = 'measure_conversion';
ALTER TABLE stock_movements ENABLE TRIGGER trg_stock_movements_append_only;
ALTER TABLE stock_movements DROP CONSTRAINT chk_stock_movements_type;
ALTER TABLE stock_movements ADD CONSTRAINT chk_stock_movements_type CHECK (type IN (
    'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore', 'order_out', 'received',
    'order_cancel_restore', 'consumption', 'partial_consumption', 'return', 'sale', 'count_adjustment', 'void',
    'external_outbound', 'reclassification', 'split'));

DO $$
DECLARE def text;
BEGIN
    def := pg_get_functiondef('service_items_check'::regproc);
    def := replace(def,
        $r$IF NEW.kind = 'partial' AND (u_kind <> 'serial' OR NOT EXISTS (
        SELECT 1 FROM units ru WHERE ru.id = NEW.unit_id AND ru.initial_meters IS NOT NULL)) THEN -- TEC-1008 roll unit$r$,
        $r$IF NEW.kind = 'partial' AND (u_kind <> 'serial' OR p_unit_type <> 'roll_meter') THEN$r$);
    EXECUTE def;
END $$;

CREATE OR REPLACE FUNCTION units_check_integrity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type    TEXT;
    org_brand   BIGINT;
    p_unit_type TEXT;
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

    SELECT unit_type, uses_fixed_barcode INTO p_unit_type, p_fixed
    FROM products WHERE id = NEW.product_id;
    IF p_fixed IS DISTINCT FROM (NEW.unit_kind = 'fixed') THEN
        RAISE EXCEPTION 'unit kind % does not match product % (uses_fixed_barcode=%)',
            NEW.unit_kind, NEW.product_id, p_fixed
            USING ERRCODE = 'check_violation';
    END IF;
    IF (p_unit_type = 'roll_meter') IS DISTINCT FROM (NEW.initial_meters IS NOT NULL) THEN
        RAISE EXCEPTION 'unit meters must be set exactly for roll_meter products (product %)',
            NEW.product_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

DROP INDEX IF EXISTS idx_units_lot;
ALTER TABLE units
    DROP CONSTRAINT IF EXISTS chk_units_note,
    DROP CONSTRAINT IF EXISTS chk_units_lot,
    DROP COLUMN IF EXISTS note,
    DROP COLUMN IF EXISTS lot;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS chk_products_waste,
    DROP CONSTRAINT IF EXISTS chk_products_min_cut,
    DROP CONSTRAINT IF EXISTS chk_products_roll_width,
    DROP CONSTRAINT IF EXISTS chk_products_roll_meters,
    DROP COLUMN IF EXISTS waste_pct,
    DROP COLUMN IF EXISTS min_cut_meters,
    DROP COLUMN IF EXISTS roll_width_cm,
    DROP COLUMN IF EXISTS default_roll_meters;
