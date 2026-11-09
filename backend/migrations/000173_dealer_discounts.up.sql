-- TEC-1064 (RV-5.3): per-buyer discounts agreed by the seller and the
-- discounted / gift order lines.
--
-- organizations: the discount the seller (center or distributor) agreed
-- with this buyer organization, kept on the buyer's commercial profile like
-- the invoice fields (000124). standard_discount_pct applies to every
-- order; first_order_discount_pct replaces it on the buyer's first order
-- (no earlier non-draft, non-cancelled order to the same seller). NULL =
-- no discount. Columns only: no foreign key, so the org merge (000152) has
-- nothing to defer.
ALTER TABLE organizations
    ADD COLUMN standard_discount_pct    NUMERIC(5,2) NULL,
    ADD COLUMN first_order_discount_pct NUMERIC(5,2) NULL,
    ADD CONSTRAINT chk_organizations_standard_discount_pct
        CHECK (standard_discount_pct IS NULL OR (standard_discount_pct >= 0 AND standard_discount_pct <= 100)),
    ADD CONSTRAINT chk_organizations_first_order_discount_pct
        CHECK (first_order_discount_pct IS NULL OR (first_order_discount_pct >= 0 AND first_order_discount_pct <= 100));

-- order_items: list_unit_price is the price before the discount (NULL when
-- no list price was found), discount_pct the applied discount (NULL = none,
-- also on a typed price or a gift) and unit_price stays the price actually
-- charged.
-- A gift line (is_gift, e.g. the plexi set) is charged 0.
ALTER TABLE order_items
    ADD COLUMN list_unit_price NUMERIC(14,4) NULL,
    ADD COLUMN discount_pct    NUMERIC(5,2)  NULL,
    ADD COLUMN is_gift         BOOLEAN       NOT NULL DEFAULT false,
    ADD CONSTRAINT chk_order_items_list_unit_price
        CHECK (list_unit_price IS NULL OR list_unit_price >= 0),
    ADD CONSTRAINT chk_order_items_discount_pct
        CHECK (discount_pct IS NULL OR (discount_pct >= 0 AND discount_pct <= 100)),
    ADD CONSTRAINT chk_order_items_gift
        CHECK (NOT is_gift OR (unit_price = 0 AND line_total = 0 AND discount_pct IS NULL));
