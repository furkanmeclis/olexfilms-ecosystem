-- TEC-1006: the warehouse-layer trace of a unit (view unit_traces).

-- name: GetUnitTrace :one
SELECT t.source::text AS source, w.uuid AS warehouse_uuid, w.code AS warehouse_code, w.name AS warehouse_name
FROM unit_traces t
LEFT JOIN warehouses w ON w.id = t.origin_warehouse_id
WHERE t.unit_id = sqlc.arg(unit_id);

-- name: ListUnitTraceOrders :many
SELECT o.uuid, o.order_no, o.status, o.external_reference, o.submitted_at, o.shipped_at, o.received_at, o.created_at,
       b.id AS buyer_id, b.uuid AS buyer_uuid, b.name AS buyer_name, b.type AS buyer_type,
       s.id AS seller_id, s.uuid AS seller_uuid, s.name AS seller_name, s.type AS seller_type
FROM order_item_units oiu
JOIN order_items oi ON oi.id = oiu.order_item_id
JOIN orders o ON o.id = oi.order_id
JOIN organizations b ON b.id = o.buyer_org_id
JOIN organizations s ON s.id = o.seller_org_id
WHERE oiu.unit_id = sqlc.arg(unit_id)
ORDER BY o.created_at, o.id;

-- name: ListUnitTraceServices :many
SELECT s.uuid, s.service_no, s.status, s.completed_at, s.created_at,
       o.id AS organization_id, o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       u.uuid AS customer_uuid, u.name AS customer_name, u.surname AS customer_surname
FROM service_items si
JOIN services s ON s.id = si.service_id
JOIN organizations o ON o.id = s.organization_id
JOIN users u ON u.id = s.customer_user_id
WHERE si.unit_id = sqlc.arg(unit_id)
GROUP BY s.id, o.id, u.id
ORDER BY s.created_at, s.id;

-- name: ListWarehouseTraceUnits :many
-- TEC-1006: every unit (any brand, K20) whose origin is the warehouse, in it or gone (state
-- in: placed in one of its locations; left: anywhere else). Sort:
-- docs/list-contract.md, keys from stock usecase WarehouseUnitSort.
SELECT u.uuid, u.barcode, u.unit_kind, u.status, u.initial_meters, u.remaining_meters, u.created_at,
       p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       t.source::text AS source, COALESCE(t.owner_type, '')::text AS owner_type,
       COALESCE(loc.warehouse_id = sqlc.arg(warehouse_id)::bigint, FALSE)::bool AS in_warehouse,
       loc.full_code AS location_code,
       h.uuid AS holder_uuid, h.name AS holder_name, h.type AS holder_type,
       o.uuid AS order_uuid, o.order_no, ob.name AS order_buyer_name,
       s.uuid AS service_uuid, s.service_no, s.completed_at AS service_completed_at,
       COALESCE(btrim(cu.name || ' ' || cu.surname), '')::text AS customer_name,
       COALESCE(st.updated_at, u.updated_at) AS updated_at
FROM unit_traces t
JOIN units u ON u.id = t.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN unit_current_state st ON st.unit_id = u.id
LEFT JOIN warehouse_locations loc ON t.owner_type = 'warehouse_location' AND loc.id = t.owner_id
LEFT JOIN organizations h ON h.id = t.holder_org_id
LEFT JOIN orders o ON o.id = t.last_order_id
LEFT JOIN organizations ob ON ob.id = o.buyer_org_id
LEFT JOIN services s ON s.id = t.last_service_id
LEFT JOIN users cu ON cu.id = s.customer_user_id
WHERE t.origin_warehouse_id = sqlc.arg(warehouse_id)::bigint
  AND (sqlc.narg(state)::text IS NULL
       OR (sqlc.narg(state)::text = 'in') = (loc.id IS NOT NULL AND loc.warehouse_id = sqlc.arg(warehouse_id)::bigint))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR t.source = ANY (sqlc.narg(sources)::text[]))
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(q)::text IS NULL
       OR u.barcode ILIKE sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.order_no ILIKE sqlc.narg(q)::text || '%'
       OR s.service_no ILIKE sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'barcode' THEN u.barcode WHEN 'product' THEN p.name WHEN 'source' THEN t.source::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'barcode' THEN u.barcode WHEN 'product' THEN p.name WHEN 'source' THEN t.source::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN u.created_at WHEN 'updated_at' THEN COALESCE(st.updated_at, u.updated_at) END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN u.created_at WHEN 'updated_at' THEN COALESCE(st.updated_at, u.updated_at) END
  END DESC,
  u.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountWarehouseTraceUnits :one
SELECT count(*)
FROM unit_traces t
JOIN units u ON u.id = t.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN warehouse_locations loc ON t.owner_type = 'warehouse_location' AND loc.id = t.owner_id
LEFT JOIN orders o ON o.id = t.last_order_id
LEFT JOIN services s ON s.id = t.last_service_id
WHERE t.origin_warehouse_id = sqlc.arg(warehouse_id)::bigint
  AND (sqlc.narg(state)::text IS NULL
       OR (sqlc.narg(state)::text = 'in') = (loc.id IS NOT NULL AND loc.warehouse_id = sqlc.arg(warehouse_id)::bigint))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR t.source = ANY (sqlc.narg(sources)::text[]))
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(q)::text IS NULL
       OR u.barcode ILIKE sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.order_no ILIKE sqlc.narg(q)::text || '%'
       OR s.service_no ILIKE sqlc.narg(q)::text || '%');
