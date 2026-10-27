-- TEC-1028: the seller warehouse an order is fulfilled from.
--
-- seller_warehouse_location_id (000049) names a single location (bin) and
-- stays as it is; the order now falls into a whole warehouse of the seller.
-- It is set at approval: the warehouse chosen when the center opens the
-- order for a dealer (TEC-1024), else the seller's primary warehouse
-- (TEC-1006). Picking suggestions and pick locations are limited to it, and
-- its staff (warehouse_members, orders.ship) are told about the new order.
-- The warehouse belongs to the seller (composite FK, like the location).
ALTER TABLE orders ADD COLUMN seller_warehouse_id BIGINT NULL;

ALTER TABLE orders ADD CONSTRAINT fk_orders_seller_warehouse
    FOREIGN KEY (seller_warehouse_id, seller_org_id)
    REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT;

CREATE INDEX idx_orders_seller_warehouse ON orders (seller_warehouse_id)
    WHERE seller_warehouse_id IS NOT NULL;

-- Open approved orders fall into the seller's primary warehouse.
UPDATE orders o
SET seller_warehouse_id = w.id
FROM warehouses w
WHERE w.organization_id = o.seller_org_id
  AND w.is_primary
  AND o.seller_warehouse_id IS NULL
  AND o.status IN ('approved', 'preparing', 'processing', 'ready');
