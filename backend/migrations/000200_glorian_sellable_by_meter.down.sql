-- Back to 000196: a synced product is not sold by the meter. A synced
-- product switched on meanwhile is switched off first.
UPDATE products SET sellable_by_meter = false
WHERE sellable_by_meter AND connection_id IS NOT NULL;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS chk_products_sellable_by_meter,
    ADD CONSTRAINT chk_products_sellable_by_meter CHECK (
        NOT sellable_by_meter
        OR (unit_type = 'piece' AND NOT uses_fixed_barcode
            AND default_roll_meters IS NOT NULL AND connection_id IS NULL)
    );
