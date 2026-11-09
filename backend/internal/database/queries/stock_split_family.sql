-- TEC-1078: the split family of a roll — the roll (depth 0) and every unit
-- cut from it, recursively through stock_splits (a part may be cut again).
-- Each row carries the unit, its product, its current holder and owner (no
-- state yet: NULL), the location when it sits in a bin, the meters of the
-- split that made it, its source barcode and the latest order that carried
-- it. The caller filters the rows by the stock.read reach of the holder.

-- name: ListUnitSplitFamily :many
WITH RECURSIVE fam AS (
    SELECT sqlc.arg(unit_id)::bigint AS unit_id, NULL::bigint AS split_id, NULL::bigint AS source_unit_id,
           NULL::numeric AS split_meters, NULL::timestamptz AS split_at, 0 AS depth
    UNION ALL
    SELECT ss.new_unit_id, ss.id, ss.source_unit_id, ss.meters::numeric, ss.created_at, fam.depth + 1
    FROM stock_splits ss
    JOIN fam ON ss.source_unit_id = fam.unit_id
    WHERE fam.depth < 16
)
SELECT fam.depth, fam.split_meters, fam.split_at,
       u.id, u.uuid, u.barcode, u.unit_kind, u.status, u.organization_id, u.brand_id,
       u.initial_meters, u.remaining_meters, u.updated_at AS unit_updated_at,
       src.uuid AS source_uuid, src.barcode AS source_barcode,
       p.id AS product_id, p.uuid AS product_uuid, p.sku, p.name AS product_name,
       p.unit_type, p.uses_fixed_barcode,
       c.uuid AS category_uuid, c.name AS category_name,
       st.owner_type, st.owner_id, st.holder_org_id, st.updated_at AS state_updated_at,
       ho.uuid AS holder_uuid, ho.name AS holder_name, ho.type AS holder_type,
       l.uuid AS location_uuid, l.code AS location_code, l.name AS location_name,
       COALESCE(ord.uuid::text, '')::text AS order_uuid, COALESCE(ord.order_no, '')::text AS order_no,
       COALESCE(ord.buyer_org_id, 0)::bigint AS order_buyer_id, COALESCE(ord.seller_org_id, 0)::bigint AS order_seller_id
FROM fam
JOIN units u ON u.id = fam.unit_id
JOIN products p ON p.id = u.product_id
JOIN product_categories c ON c.id = p.category_id
LEFT JOIN units src ON src.id = fam.source_unit_id
LEFT JOIN unit_current_state st ON st.unit_id = u.id
LEFT JOIN organizations ho ON ho.id = st.holder_org_id
LEFT JOIN warehouse_locations l ON st.owner_type = 'warehouse_location' AND l.id = st.owner_id
LEFT JOIN LATERAL (
    SELECT o.uuid, o.order_no, o.buyer_org_id, o.seller_org_id
    FROM order_item_units oiu
    JOIN order_items oi ON oi.id = oiu.order_item_id
    JOIN orders o ON o.id = oi.order_id
    WHERE oiu.unit_id = u.id
    ORDER BY o.created_at DESC, o.id DESC
    LIMIT 1
) ord ON TRUE
ORDER BY fam.depth, fam.split_at, u.id
LIMIT 500;

-- name: GetUnitSplitSource :one
-- The roll a unit was cut from (TEC-1078), with its current holder.
SELECT ss.meters, src.id, src.uuid, src.barcode, src.brand_id, src.organization_id,
       st.holder_org_id
FROM stock_splits ss
JOIN units src ON src.id = ss.source_unit_id
LEFT JOIN unit_current_state st ON st.unit_id = src.id
WHERE ss.new_unit_id = sqlc.arg(unit_id);
