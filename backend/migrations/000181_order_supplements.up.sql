-- TEC-1222 (revize 5.9): supplementary orders. A shipped or delivered order
-- may get a follow-up order (plexi and sign sets sent later) that copies its
-- seller, buyer, brand and seller warehouse and runs its own flow (approval,
-- preparing, shipping, delivery, books, invoice).
--
-- * parent_order_id: the main order. Same seller organization and brand
--   (composite FK to uq_orders_id_org_brand). orders is rewritten by an
--   organization merge (TEC-1003), so the FK is deferred. A supplement is
--   never the parent of another one (checked by the use case: the main order
--   of a supplement's supplement is the main order).
ALTER TABLE orders
    ADD COLUMN parent_order_id BIGINT NULL,
    ADD CONSTRAINT fk_orders_parent_order FOREIGN KEY (parent_order_id, organization_id, brand_id)
        REFERENCES orders (id, organization_id, brand_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT chk_orders_parent_not_self CHECK (parent_order_id IS NULL OR parent_order_id <> id);

CREATE INDEX idx_orders_parent_order ON orders (parent_order_id)
    WHERE parent_order_id IS NOT NULL;
