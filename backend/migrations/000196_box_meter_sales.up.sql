-- TEC-1238 (kutu/metraj 1/7, kararlar TEC-1236 K1-K14): the same product is
-- sold as a closed box or by the meter; the order line chooses.
--
-- 1. products.sellable_by_meter (K1): the product card's "sold by the
--    meter" switch. Only a serial piece product (no fixed barcode) with a
--    box length (default_roll_meters) may be sold by the meter, and never a
--    product synced from Glorian (connection_id). roll_meter products are
--    always sold by the meter (implicit; the switch stays false).
ALTER TABLE products
    ADD COLUMN sellable_by_meter BOOLEAN NOT NULL DEFAULT false,
    ADD CONSTRAINT chk_products_sellable_by_meter CHECK (
        NOT sellable_by_meter
        OR (unit_type = 'piece' AND NOT uses_fixed_barcode
            AND default_roll_meters IS NOT NULL AND connection_id IS NULL)
    );

-- 2. product_prices.meter_price (K2): the center's meter list price next to
--    its box list price (sale_to_distributor_price), per currency. NULL =
--    computed: the buyer's box price / the box length. A buyer's meter price
--    carries the same ratio to its box price as the list prices (no
--    separate distributor / dealer meter columns).
ALTER TABLE product_prices
    ADD COLUMN meter_price NUMERIC(14,4) NULL,
    ADD CONSTRAINT chk_product_prices_meter CHECK (meter_price IS NULL OR meter_price >= 0);

-- 3. order_items.sale_measure: the line's own measure. A box line counts
--    quantity, a meter line meters; the column is derived from the amount
--    so every existing line is backfilled as it behaves today (roll_meter
--    lines carry meters -> 'meter', the others 'box').
ALTER TABLE order_items
    ADD COLUMN sale_measure VARCHAR(8) GENERATED ALWAYS AS (
        CASE WHEN meters IS NULL THEN 'box' ELSE 'meter' END
    ) STORED;

-- K10: one line per product and measure (1 box + 3 m of the same product).
ALTER TABLE order_items DROP CONSTRAINT uq_order_items_order_product;
ALTER TABLE order_items
    ADD CONSTRAINT uq_order_items_order_product_measure UNIQUE (order_id, product_id, sale_measure);

-- 4. order_items.line_credits (K3/K12): the usage credits each cut piece of
--    a meter line carries (default 5, asked on every meter line); the
--    plotter mark of a meter line shares this field. Required on a meter
--    line of a piece product (checked by the trigger below), NULL on box
--    lines and on roll_meter lines (rolls keep their meter consumption).
ALTER TABLE order_items
    ADD COLUMN line_credits INT NULL,
    ADD CONSTRAINT chk_order_items_line_credits CHECK (
        line_credits IS NULL OR (meters IS NOT NULL AND line_credits BETWEEN 1 AND 1000)
    );

-- 5. The amount rule reads the line's measure, not only the product type:
--    a roll_meter product is ordered in meters; a piece product by quantity
--    (box) or — without a fixed barcode — in meters with line credits. The
--    sellable_by_meter switch is checked by the use case (a draft keeps its
--    meter line after the switch is turned off).
CREATE OR REPLACE FUNCTION order_items_check_amount() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    p_unit_type TEXT;
    p_fixed     BOOLEAN;
BEGIN
    SELECT unit_type, uses_fixed_barcode INTO p_unit_type, p_fixed
    FROM products WHERE id = NEW.product_id;
    IF p_unit_type = 'roll_meter' THEN
        IF NEW.meters IS NULL THEN
            RAISE EXCEPTION 'order line of product % must use meters', NEW.product_id
                USING ERRCODE = 'check_violation';
        END IF;
        IF NEW.line_credits IS NOT NULL THEN
            RAISE EXCEPTION 'order line of roll product % carries no credits', NEW.product_id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.meters IS NOT NULL THEN
        IF p_fixed THEN
            RAISE EXCEPTION 'order line of fixed barcode product % must use quantity', NEW.product_id
                USING ERRCODE = 'check_violation';
        END IF;
        IF NEW.line_credits IS NULL THEN
            RAISE EXCEPTION 'meter line of product % needs line credits', NEW.product_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER trg_order_items_check_amount ON order_items;
CREATE TRIGGER trg_order_items_check_amount
    BEFORE INSERT OR UPDATE OF product_id, quantity, meters, line_credits ON order_items
    FOR EACH ROW
    EXECUTE FUNCTION order_items_check_amount();

-- 6. unit_credit_default: a unit with meters (a cut piece) carries the
--    credits of the meter line it was shipped on (line_credits); without
--    such a line it carries none, as before. Pieces without meters keep the
--    TEC-1221 rule (plotter credits, else the product default).
CREATE OR REPLACE FUNCTION unit_credit_default(p_unit_id BIGINT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN u.unit_kind <> 'serial' THEN NULL
        WHEN u.initial_meters IS NOT NULL THEN (
            SELECT oi.line_credits
            FROM order_item_units oiu
            JOIN order_items oi ON oi.id = oiu.order_item_id
            WHERE oiu.unit_id = u.id AND oi.line_credits IS NOT NULL
            ORDER BY oiu.assigned_at DESC, oiu.id DESC
            LIMIT 1
        )
        ELSE COALESCE((
            SELECT oi.plotter_credits
            FROM order_item_units oiu
            JOIN order_items oi ON oi.id = oiu.order_item_id
            WHERE oiu.unit_id = u.id AND oi.plotter_cut AND oi.plotter_credits IS NOT NULL
            ORDER BY oiu.assigned_at DESC, oiu.id DESC
            LIMIT 1
        ), p.usage_credits)
    END
    FROM units u
    JOIN products p ON p.id = u.product_id
    WHERE u.id = p_unit_id
$$;
