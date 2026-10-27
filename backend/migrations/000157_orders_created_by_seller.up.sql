-- TEC-1024: the center opens an order on behalf of a dealer directly under
-- it. The order starts approved; created_by_seller marks it. Lines may carry
-- a price typed by the center (manual) or no price at all (missing, 0).
ALTER TABLE orders ADD COLUMN created_by_seller BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE order_items DROP CONSTRAINT chk_order_items_price_source;
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_price_source
    CHECK (price_source IN ('list', 'override', 'distributor_dealer', 'manual', 'missing'));
