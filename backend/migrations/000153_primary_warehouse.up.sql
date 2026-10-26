-- TEC-1006: the primary warehouse of an organization. For the Olex center it
-- is "AFM Florya Merkez" (the legacy AFM Florya warehouse, the center's own
-- service point since TEC-1003): legacy units that never touched a
-- warehouse location (hub center stock, units shipped to dealers, units used
-- in services) originate from it in the warehouse layer.
ALTER TABLE warehouses ADD COLUMN is_primary BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX uq_warehouses_primary ON warehouses (organization_id) WHERE is_primary;

-- The warehouse-layer trace of a unit (TEC-1006), derived on read:
--   origin_warehouse_id  the warehouse of the first location it was placed
--                        in, else the issuer's primary warehouse (legacy hub
--                        units never touched a location);
--   last_order_id        the latest order line that carried it;
--   last_service_id      the latest service that used it;
--   source               'order'   it went out with an order,
--                        'stock'   it never left the issuer,
--                        'unknown' it is (or was used) at another organization
--                                  without an order ("kaynak bilinmiyor").
CREATE VIEW unit_traces AS
SELECT u.id AS unit_id,
       u.brand_id,
       u.organization_id AS issuer_org_id,
       COALESCE(fl.warehouse_id, pw.id) AS origin_warehouse_id,
       st.owner_type,
       st.owner_id,
       COALESCE(st.holder_org_id, ls.organization_id) AS holder_org_id,
       lo.order_id AS last_order_id,
       ls.service_id AS last_service_id,
       CASE
           WHEN lo.order_id IS NOT NULL THEN 'order'
           WHEN COALESCE(st.holder_org_id, ls.organization_id, u.organization_id) = u.organization_id THEN 'stock'
           ELSE 'unknown'
       END AS source
FROM units u
LEFT JOIN unit_current_state st ON st.unit_id = u.id
LEFT JOIN warehouses pw ON pw.organization_id = u.organization_id AND pw.is_primary
LEFT JOIN LATERAL (
    SELECT l.warehouse_id
    FROM stock_movements m
    JOIN warehouse_locations l ON l.id = m.to_owner_id
    WHERE m.unit_id = u.id AND m.to_owner_type = 'warehouse_location'
    ORDER BY m.created_at, m.id
    LIMIT 1
) fl ON TRUE
LEFT JOIN LATERAL (
    SELECT oi.order_id
    FROM order_item_units oiu
    JOIN order_items oi ON oi.id = oiu.order_item_id
    WHERE oiu.unit_id = u.id
    ORDER BY oiu.id DESC
    LIMIT 1
) lo ON TRUE
LEFT JOIN LATERAL (
    SELECT s.id AS service_id, s.organization_id
    FROM service_items si
    JOIN services s ON s.id = si.service_id
    WHERE si.unit_id = u.id AND s.status <> 'cancelled'
    ORDER BY s.completed_at DESC NULLS LAST, s.id DESC
    LIMIT 1
) ls ON TRUE;
