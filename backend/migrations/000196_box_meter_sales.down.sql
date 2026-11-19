-- Restores the TEC-1221 credit default (000182) and the product-type amount
-- rule (000049). The one-line-per-product unique fails while an order holds
-- a box and a meter line of the same product; merge or remove them first.
CREATE OR REPLACE FUNCTION unit_credit_default(p_unit_id BIGINT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN u.unit_kind <> 'serial' OR u.initial_meters IS NOT NULL THEN NULL
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

DROP TRIGGER IF EXISTS trg_order_items_check_amount ON order_items;

CREATE OR REPLACE FUNCTION order_items_check_amount() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    p_unit_type TEXT;
BEGIN
    SELECT unit_type INTO p_unit_type FROM products WHERE id = NEW.product_id;
    IF (p_unit_type = 'roll_meter') IS DISTINCT FROM (NEW.meters IS NOT NULL) THEN
        RAISE EXCEPTION 'order line of product % must use %', NEW.product_id,
            CASE WHEN p_unit_type = 'roll_meter' THEN 'meters' ELSE 'quantity' END
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_order_items_check_amount
    BEFORE INSERT OR UPDATE OF product_id, quantity, meters ON order_items
    FOR EACH ROW
    EXECUTE FUNCTION order_items_check_amount();

ALTER TABLE order_items
    DROP CONSTRAINT IF EXISTS chk_order_items_line_credits,
    DROP COLUMN IF EXISTS line_credits;

ALTER TABLE order_items DROP CONSTRAINT IF EXISTS uq_order_items_order_product_measure;
ALTER TABLE order_items
    ADD CONSTRAINT uq_order_items_order_product UNIQUE (order_id, product_id);
ALTER TABLE order_items DROP COLUMN IF EXISTS sale_measure;

ALTER TABLE product_prices
    DROP CONSTRAINT IF EXISTS chk_product_prices_meter,
    DROP COLUMN IF EXISTS meter_price;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS chk_products_sellable_by_meter,
    DROP COLUMN IF EXISTS sellable_by_meter;
