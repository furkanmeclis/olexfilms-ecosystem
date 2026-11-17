-- TEC-1229: stock view columns, movement list, reorder rules, aging and
-- end-of-day balances. Nothing here reads or returns a price except the
-- aging report value, which the report gates with a pricing permission.

-- name: ListStockReservedByProduct :many
-- Active order reservations on stock the organization holds, per product.
-- warehouse_id / location_id narrow to the units placed there. Serial
-- pieces add their quantity, rolls their reserved meters. A fixed barcode
-- reservation is capped by the holding in the scope (it names no
-- location).
WITH serial AS (
    SELECT u.product_id,
           COALESCE(SUM(r.quantity), 0)::bigint AS quantity,
           COALESCE(SUM(r.meters), 0)::numeric(14,2) AS meters
    FROM stock_reservations r
    JOIN units u ON u.id = r.unit_id
    JOIN unit_current_state st ON st.unit_id = r.unit_id
    LEFT JOIN warehouse_locations wl ON wl.id = st.owner_location_id
    WHERE r.status = 'active'
      AND r.unit_kind = 'serial'
      AND st.holder_org_id = sqlc.arg(organization_id)::bigint
      AND st.status IN ('available', 'placed')
      AND (sqlc.narg(warehouse_id)::bigint IS NULL OR wl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
      AND (sqlc.narg(location_id)::bigint IS NULL OR st.owner_location_id = sqlc.narg(location_id)::bigint)
      AND u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
    GROUP BY u.product_id
), fixed_reserved AS (
    SELECT r.unit_id, SUM(r.quantity)::bigint AS quantity
    FROM stock_reservations r
    WHERE r.status = 'active'
      AND r.unit_kind = 'fixed'
      AND r.organization_id = sqlc.arg(organization_id)::bigint
    GROUP BY r.unit_id
), fixed_held AS (
    SELECT h.unit_id, SUM(h.quantity_on_hand)::bigint AS quantity
    FROM fixed_barcode_holdings h
    LEFT JOIN warehouse_locations wl ON wl.id = h.owner_location_id
    WHERE h.holder_org_id = sqlc.arg(organization_id)::bigint
      AND h.owner_type IN ('warehouse_location', 'organization')
      AND (sqlc.narg(warehouse_id)::bigint IS NULL OR wl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
      AND (sqlc.narg(location_id)::bigint IS NULL OR h.owner_location_id = sqlc.narg(location_id)::bigint)
    GROUP BY h.unit_id
), fixed AS (
    SELECT u.product_id, SUM(LEAST(fr.quantity, fh.quantity))::bigint AS quantity
    FROM fixed_reserved fr
    JOIN fixed_held fh ON fh.unit_id = fr.unit_id
    JOIN units u ON u.id = fr.unit_id
    WHERE u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
    GROUP BY u.product_id
)
SELECT p.product_id,
       (COALESCE(s.quantity, 0) + COALESCE(f.quantity, 0))::bigint AS reserved_quantity,
       COALESCE(s.meters, 0)::numeric(14,2)::text AS reserved_meters
FROM (SELECT product_id FROM serial UNION SELECT product_id FROM fixed) p
LEFT JOIN serial s ON s.product_id = p.product_id
LEFT JOIN fixed f ON f.product_id = p.product_id;

-- name: ListStockIncomingByProduct :many
-- Stock on its way to the organization, per product: in-transit warehouse
-- transfers of the organization (to warehouse_id / location_id when given)
-- and shipped orders bought by it (to the buyer location's warehouse /
-- the buyer location when given). Serial units count one each (rolls add
-- their remaining meters); fixed barcodes their line quantity.
WITH transfer_units AS (
    SELECT l.unit_id, l.quantity
    FROM warehouse_transfer_lines l
    JOIN warehouse_transfers t ON t.id = l.transfer_id
    WHERE t.organization_id = sqlc.arg(organization_id)::bigint
      AND t.status = 'in_transit'
      AND l.is_open
      AND (sqlc.narg(warehouse_id)::bigint IS NULL OR t.to_warehouse_id = sqlc.narg(warehouse_id)::bigint)
      AND (sqlc.narg(location_id)::bigint IS NULL
           OR COALESCE(l.target_location_id, t.to_location_id) = sqlc.narg(location_id)::bigint)
), order_units AS (
    SELECT oiu.unit_id, COALESCE(oiu.quantity, 1) AS quantity
    FROM order_item_units oiu
    JOIN order_items oi ON oi.id = oiu.order_item_id
    JOIN orders o ON o.id = oi.order_id
    LEFT JOIN warehouse_locations bl ON bl.id = o.buyer_warehouse_location_id
    WHERE o.buyer_org_id = sqlc.arg(organization_id)::bigint
      AND o.status IN ('shipped', 'delivered')
      AND oiu.movement_id IS NOT NULL
      AND (sqlc.narg(warehouse_id)::bigint IS NULL OR bl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
      AND (sqlc.narg(location_id)::bigint IS NULL OR o.buyer_warehouse_location_id = sqlc.narg(location_id)::bigint)
), incoming AS (
    SELECT unit_id, quantity FROM transfer_units
    UNION ALL
    SELECT unit_id, quantity FROM order_units
)
SELECT u.product_id,
       SUM(CASE WHEN u.unit_kind = 'fixed' THEN i.quantity ELSE 1 END)::bigint AS incoming_quantity,
       COALESCE(SUM(CASE WHEN u.unit_kind = 'serial' THEN u.remaining_meters END), 0)::numeric(14,2)::text AS incoming_meters
FROM incoming i
JOIN units u ON u.id = i.unit_id
LEFT JOIN unit_current_state st ON st.unit_id = u.id
WHERE u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
  AND (u.unit_kind = 'fixed' OR st.status = 'in_transit')
GROUP BY u.product_id;

-- name: ListStockMovementRows :many
-- Ledger movements of the organization (recorded on it, landing on one of
-- its locations or on the organization itself). warehouse_id narrows to
-- movements leaving or reaching a location of that warehouse. Filters:
-- types (any of), product, actor, created_from inclusive / created_to
-- exclusive, q (barcode, SKU or product name). Sort keys: created_at,
-- type, product; id tiebreak.
SELECT m.id, m.uuid, m.type, m.quantity_delta, m.meters_delta::text AS meters_delta,
       m.from_owner_type, m.to_owner_type, m.from_status, m.to_status,
       m.reference_type, m.reason, m.created_at,
       u.uuid AS unit_uuid, u.barcode, u.unit_kind,
       p.uuid AS product_uuid, p.sku, p.name AS product_name, p.unit_type,
       fl.uuid AS from_location_uuid, fl.full_code AS from_location_code,
       fw.uuid AS from_warehouse_uuid, fw.name AS from_warehouse_name,
       tl.uuid AS to_location_uuid, tl.full_code AS to_location_code,
       tw.uuid AS to_warehouse_uuid, tw.name AS to_warehouse_name,
       fo.uuid AS from_org_uuid, fo.name AS from_org_name,
       too.uuid AS to_org_uuid, too.name AS to_org_name,
       au.uuid AS actor_uuid,
       btrim(COALESCE(au.name, '') || ' ' || COALESCE(au.surname, ''))::text AS actor_name
FROM stock_movements m
JOIN units u ON u.id = m.unit_id
JOIN products p ON p.id = m.product_id
LEFT JOIN warehouse_locations fl ON m.from_owner_type = 'warehouse_location' AND fl.id = m.from_owner_id
LEFT JOIN warehouses fw ON fw.id = fl.warehouse_id
LEFT JOIN warehouse_locations tl ON m.to_owner_type = 'warehouse_location' AND tl.id = m.to_owner_id
LEFT JOIN warehouses tw ON tw.id = tl.warehouse_id
LEFT JOIN organizations fo ON m.from_owner_type IN ('organization', 'trash') AND fo.id = m.from_owner_id
LEFT JOIN organizations too ON m.to_owner_type IN ('organization', 'trash') AND too.id = m.to_owner_id
LEFT JOIN users au ON au.id = m.actor_user_id
WHERE (m.organization_id = sqlc.arg(organization_id)::bigint
       OR tl.organization_id = sqlc.arg(organization_id)::bigint
       OR (m.to_owner_type = 'organization' AND m.to_owner_id = sqlc.arg(organization_id)::bigint))
  AND (sqlc.narg(warehouse_id)::bigint IS NULL
       OR (fl.organization_id = sqlc.arg(organization_id)::bigint AND fl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
       OR (tl.organization_id = sqlc.arg(organization_id)::bigint AND tl.warehouse_id = sqlc.narg(warehouse_id)::bigint))
  AND (COALESCE(cardinality(sqlc.narg(types)::text[]), 0) = 0 OR m.type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(product_id)::bigint IS NULL OR m.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(actor_user_id)::bigint IS NULL OR m.actor_user_id = sqlc.narg(actor_user_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR m.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR m.created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'type' THEN m.type WHEN 'product' THEN p.name END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'type' THEN m.type WHEN 'product' THEN p.name END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN m.id END DESC,
  m.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStockMovementRows :one
SELECT COUNT(*)::bigint
FROM stock_movements m
JOIN units u ON u.id = m.unit_id
JOIN products p ON p.id = m.product_id
LEFT JOIN warehouse_locations fl ON m.from_owner_type = 'warehouse_location' AND fl.id = m.from_owner_id
LEFT JOIN warehouse_locations tl ON m.to_owner_type = 'warehouse_location' AND tl.id = m.to_owner_id
WHERE (m.organization_id = sqlc.arg(organization_id)::bigint
       OR tl.organization_id = sqlc.arg(organization_id)::bigint
       OR (m.to_owner_type = 'organization' AND m.to_owner_id = sqlc.arg(organization_id)::bigint))
  AND (sqlc.narg(warehouse_id)::bigint IS NULL
       OR (fl.organization_id = sqlc.arg(organization_id)::bigint AND fl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
       OR (tl.organization_id = sqlc.arg(organization_id)::bigint AND tl.warehouse_id = sqlc.narg(warehouse_id)::bigint))
  AND (COALESCE(cardinality(sqlc.narg(types)::text[]), 0) = 0 OR m.type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(product_id)::bigint IS NULL OR m.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(actor_user_id)::bigint IS NULL OR m.actor_user_id = sqlc.narg(actor_user_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR m.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR m.created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: ListStockReorderRules :many
-- Reorder rules of the organization; warehouse / product narrow them.
SELECT r.*,
       w.uuid AS warehouse_uuid, w.code AS warehouse_code, w.name AS warehouse_name,
       p.uuid AS product_uuid, p.sku, p.name AS product_name, p.unit_type,
       c.uuid AS category_uuid, c.name AS category_name
FROM stock_reorder_rules r
JOIN warehouses w ON w.id = r.warehouse_id
JOIN products p ON p.id = r.product_id
JOIN product_categories c ON c.id = p.category_id
WHERE r.organization_id = sqlc.arg(organization_id)::bigint
  AND (sqlc.narg(warehouse_id)::bigint IS NULL OR r.warehouse_id = sqlc.narg(warehouse_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR r.product_id = sqlc.narg(product_id)::bigint)
ORDER BY w.sort_order, w.name, p.name, r.id;

-- name: GetStockReorderRuleByUUID :one
SELECT * FROM stock_reorder_rules
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: UpsertStockReorderRule :one
INSERT INTO stock_reorder_rules (
    organization_id, brand_id, warehouse_id, product_id, min_quantity, reorder_point,
    max_quantity, warning_days, cover_days, created_by_user_id, updated_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(warehouse_id), sqlc.arg(product_id),
    sqlc.arg(min_quantity), sqlc.narg(reorder_point), sqlc.narg(max_quantity),
    sqlc.narg(warning_days), sqlc.narg(cover_days), sqlc.narg(user_id), sqlc.narg(user_id)
)
ON CONFLICT ON CONSTRAINT uq_stock_reorder_rules_warehouse_product DO UPDATE
SET min_quantity = EXCLUDED.min_quantity,
    reorder_point = EXCLUDED.reorder_point,
    max_quantity = EXCLUDED.max_quantity,
    warning_days = EXCLUDED.warning_days,
    cover_days = EXCLUDED.cover_days,
    updated_by_user_id = EXCLUDED.updated_by_user_id
RETURNING *;

-- name: DeleteStockReorderRule :execrows
DELETE FROM stock_reorder_rules
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: SetStockReorderRuleAlert :exec
UPDATE stock_reorder_rules
SET alert_state = sqlc.arg(alert_state),
    alerted_at = CASE WHEN sqlc.arg(alerted)::bool THEN NOW() ELSE alerted_at END,
    evaluated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: ListStockReorderOrganizations :many
SELECT DISTINCT r.organization_id
FROM stock_reorder_rules r
JOIN organizations o ON o.id = r.organization_id
WHERE o.deleted_at IS NULL
ORDER BY r.organization_id;

-- name: ListWarehouseOnHandByProduct :many
-- On-hand totals of products in one warehouse (bin_product_stocks).
SELECT b.product_id, SUM(b.quantity)::bigint AS quantity, SUM(b.meters)::numeric(14,2)::text AS meters
FROM bin_product_stocks b
JOIN warehouse_locations wl ON wl.id = b.location_id
WHERE wl.warehouse_id = sqlc.arg(warehouse_id)::bigint
  AND b.organization_id = sqlc.arg(organization_id)::bigint
  AND b.product_id = ANY(sqlc.arg(product_ids)::bigint[])
GROUP BY b.product_id;

-- name: ListLatestForecastAvgDaily :many
-- Latest 30-day average daily use of products of the organization
-- (stock_forecasts), for the warehouse day thresholds.
SELECT f.product_id, f.avg_daily_30::numeric(14,4)::text AS avg_daily_30
FROM stock_forecasts f
WHERE f.organization_id = sqlc.arg(organization_id)::bigint
  AND f.is_latest
  AND f.product_id = ANY(sqlc.arg(product_ids)::bigint[]);

-- name: ListStockQuarantineByProduct :many
-- TEC-1229 (with TEC-1226): serial units in quarantine held by the
-- organization (warehouse_id / location_id narrow to the units there).
-- Quarantined units are not on hand; rolls add their remaining meters.
SELECT u.product_id,
       COUNT(*)::bigint AS quarantine_quantity,
       COALESCE(SUM(u.remaining_meters), 0)::numeric(14,2)::text AS quarantine_meters
FROM unit_current_state st
JOIN units u ON u.id = st.unit_id
LEFT JOIN warehouse_locations wl ON wl.id = st.owner_location_id
WHERE st.status = 'quarantine'
  AND st.holder_org_id = sqlc.arg(organization_id)::bigint
  AND (sqlc.narg(warehouse_id)::bigint IS NULL OR wl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
  AND (sqlc.narg(location_id)::bigint IS NULL OR st.owner_location_id = sqlc.narg(location_id)::bigint)
  AND u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
GROUP BY u.product_id;
