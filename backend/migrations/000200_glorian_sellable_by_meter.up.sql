-- TEC-1251 (TEC-1236 K1, 19.11): a product synced from Glorian may be sold
-- by the meter too. The check of 000196 drops its "connection_id IS NULL"
-- condition; everything else stays.
ALTER TABLE products
    DROP CONSTRAINT IF EXISTS chk_products_sellable_by_meter,
    ADD CONSTRAINT chk_products_sellable_by_meter CHECK (
        NOT sellable_by_meter
        OR (unit_type = 'piece' AND NOT uses_fixed_barcode
            AND default_roll_meters IS NOT NULL)
    );
