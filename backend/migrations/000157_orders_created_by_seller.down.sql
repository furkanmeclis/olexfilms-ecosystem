UPDATE order_items SET price_source = 'override' WHERE price_source IN ('manual', 'missing');
ALTER TABLE order_items DROP CONSTRAINT chk_order_items_price_source;
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_price_source
    CHECK (price_source IN ('list', 'override', 'distributor_dealer'));

ALTER TABLE orders DROP COLUMN created_by_seller;
