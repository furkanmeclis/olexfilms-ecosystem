ALTER TABLE order_items
    DROP CONSTRAINT chk_order_items_gift,
    DROP CONSTRAINT chk_order_items_discount_pct,
    DROP CONSTRAINT chk_order_items_list_unit_price,
    DROP COLUMN is_gift,
    DROP COLUMN discount_pct,
    DROP COLUMN list_unit_price;

ALTER TABLE organizations
    DROP CONSTRAINT chk_organizations_first_order_discount_pct,
    DROP CONSTRAINT chk_organizations_standard_discount_pct,
    DROP COLUMN first_order_discount_pct,
    DROP COLUMN standard_discount_pct;
