-- TEC-1210 (analytics A): the role reports of envelope v2. Scope arguments
-- follow reports.sql (brand_id always, org_ids NULL = whole brand, ranges
-- half open [range_from, range_to), buckets date_trunc in the caller's
-- zone).

-- orders.fulfillment (warehouse): the orders the organizations in reach
-- sell (organization_id = seller), grouped by the shipping warehouse.
-- Orders cancelled after shipping are left out of shipped / received.
-- Open = approved .. processing (not shipped yet); overdue = open and
-- approved more than 48 hours before now_at. Preparation time is approved
-- -> shipped of the orders shipped in the range (seconds; the usecase
-- divides by prep_count).

-- name: ReportFulfillmentTrend :many
SELECT 'shipped'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, o.shipped_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND o.status NOT IN ('cancelling', 'cancelled')
  AND o.shipped_at >= sqlc.arg(range_from)::timestamptz
  AND o.shipped_at < sqlc.arg(range_to)::timestamptz
GROUP BY 2
UNION ALL
SELECT 'received'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, o.received_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND o.status NOT IN ('cancelling', 'cancelled')
  AND o.received_at >= sqlc.arg(range_from)::timestamptz
  AND o.received_at < sqlc.arg(range_to)::timestamptz
GROUP BY 2;

-- name: ReportFulfillmentByWarehouse :many
-- One row per shipping warehouse and seller (warehouse_uuid NULL: no
-- warehouse set).
SELECT w.uuid AS warehouse_uuid, w.name AS warehouse_name,
       so.uuid AS organization_uuid, so.name AS organization_name,
       COUNT(*) FILTER (WHERE o.shipped_at >= sqlc.arg(range_from)::timestamptz
                          AND o.shipped_at < sqlc.arg(range_to)::timestamptz
                          AND o.status NOT IN ('cancelling', 'cancelled'))::bigint AS shipped_count,
       COUNT(*) FILTER (WHERE o.received_at >= sqlc.arg(range_from)::timestamptz
                          AND o.received_at < sqlc.arg(range_to)::timestamptz
                          AND o.status NOT IN ('cancelling', 'cancelled'))::bigint AS received_count,
       COUNT(*) FILTER (WHERE o.status IN ('approved', 'preparing', 'ready', 'processing'))::bigint AS open_count,
       COUNT(*) FILTER (WHERE o.status IN ('approved', 'preparing', 'ready', 'processing')
                          AND o.approved_at < sqlc.arg(now_at)::timestamptz - interval '48 hours')::bigint AS overdue_count,
       COUNT(*) FILTER (WHERE o.shipped_at >= sqlc.arg(range_from)::timestamptz
                          AND o.shipped_at < sqlc.arg(range_to)::timestamptz
                          AND o.status NOT IN ('cancelling', 'cancelled')
                          AND o.approved_at IS NOT NULL)::bigint AS prep_count,
       COALESCE(SUM(EXTRACT(EPOCH FROM (o.shipped_at - o.approved_at)))
                FILTER (WHERE o.shipped_at >= sqlc.arg(range_from)::timestamptz
                          AND o.shipped_at < sqlc.arg(range_to)::timestamptz
                          AND o.status NOT IN ('cancelling', 'cancelled')
                          AND o.approved_at IS NOT NULL), 0)::float8 AS prep_seconds
FROM orders o
JOIN organizations so ON so.id = o.organization_id
LEFT JOIN warehouses w ON w.id = o.seller_warehouse_id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (o.status IN ('approved', 'preparing', 'ready', 'processing')
       OR (o.shipped_at >= sqlc.arg(range_from)::timestamptz AND o.shipped_at < sqlc.arg(range_to)::timestamptz)
       OR (o.received_at >= sqlc.arg(range_from)::timestamptz AND o.received_at < sqlc.arg(range_to)::timestamptz))
GROUP BY w.id, w.uuid, w.name, so.id, so.uuid, so.name
ORDER BY shipped_count DESC, open_count DESC, lower(w.name) NULLS LAST, w.id NULLS LAST, so.id;

-- stock.movement: serial unit movements into and out of the warehouses in
-- reach (a location's warehouse). A move inside one warehouse (placement,
-- location to location) is neither in nor out; a transfer between two
-- warehouses is out of one and in to the other. The location's
-- organization is the reach (org_ids).

-- name: ReportStockMovementTrend :many
WITH m AS (
    SELECT sm.created_at, fl.warehouse_id AS from_wh, fl.organization_id AS from_org,
           tl.warehouse_id AS to_wh, tl.organization_id AS to_org
    FROM stock_movements sm
    LEFT JOIN warehouse_locations fl ON sm.from_owner_type = 'warehouse_location' AND fl.id = sm.from_owner_id
    LEFT JOIN warehouse_locations tl ON sm.to_owner_type = 'warehouse_location' AND tl.id = sm.to_owner_id
    WHERE sm.brand_id = sqlc.arg(brand_id)
      AND sm.created_at >= sqlc.arg(range_from)::timestamptz
      AND sm.created_at < sqlc.arg(range_to)::timestamptz
      AND (fl.warehouse_id IS NOT NULL OR tl.warehouse_id IS NOT NULL)
)
SELECT 'in'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, m.created_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM m
WHERE m.to_wh IS NOT NULL AND m.to_wh IS DISTINCT FROM m.from_wh
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.to_org = ANY (sqlc.narg(org_ids)::bigint[]))
GROUP BY 2
UNION ALL
SELECT 'out'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, m.created_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM m
WHERE m.from_wh IS NOT NULL AND m.from_wh IS DISTINCT FROM m.to_wh
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.from_org = ANY (sqlc.narg(org_ids)::bigint[]))
GROUP BY 2;

-- name: ReportStockMovementByWarehouse :many
WITH m AS (
    SELECT fl.warehouse_id AS from_wh, fl.organization_id AS from_org,
           tl.warehouse_id AS to_wh, tl.organization_id AS to_org
    FROM stock_movements sm
    LEFT JOIN warehouse_locations fl ON sm.from_owner_type = 'warehouse_location' AND fl.id = sm.from_owner_id
    LEFT JOIN warehouse_locations tl ON sm.to_owner_type = 'warehouse_location' AND tl.id = sm.to_owner_id
    WHERE sm.brand_id = sqlc.arg(brand_id)
      AND sm.created_at >= sqlc.arg(range_from)::timestamptz
      AND sm.created_at < sqlc.arg(range_to)::timestamptz
      AND (fl.warehouse_id IS NOT NULL OR tl.warehouse_id IS NOT NULL)
), e AS (
    SELECT m.to_wh AS wh, 1 AS in_n, 0 AS out_n
    FROM m
    WHERE m.to_wh IS NOT NULL AND m.to_wh IS DISTINCT FROM m.from_wh
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.to_org = ANY (sqlc.narg(org_ids)::bigint[]))
    UNION ALL
    SELECT m.from_wh AS wh, 0 AS in_n, 1 AS out_n
    FROM m
    WHERE m.from_wh IS NOT NULL AND m.from_wh IS DISTINCT FROM m.to_wh
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.from_org = ANY (sqlc.narg(org_ids)::bigint[]))
)
SELECT w.uuid AS warehouse_uuid, w.name AS warehouse_name,
       o.uuid AS organization_uuid, o.name AS organization_name,
       SUM(e.in_n)::bigint AS in_count, SUM(e.out_n)::bigint AS out_count
FROM e
JOIN warehouses w ON w.id = e.wh
JOIN organizations o ON o.id = w.organization_id
GROUP BY w.id, w.uuid, w.name, o.uuid, o.name
ORDER BY SUM(e.in_n) + SUM(e.out_n) DESC, lower(w.name), w.id
LIMIT sqlc.arg(row_limit);

-- network.summary: the dealers in reach with their services completed,
-- orders placed (as buyer; draft and cancelled left out) and warranties
-- started in the range. Most services first.

-- name: ReportNetworkDealers :many
SELECT d.uuid AS organization_uuid, d.name AS organization_name, d.city, d.status,
       COALESCE(sv.n, 0)::bigint AS services_count,
       COALESCE(od.n, 0)::bigint AS orders_count,
       COALESCE(wr.n, 0)::bigint AS warranties_count
FROM organizations d
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS n FROM services s
    WHERE s.organization_id = d.id AND s.brand_id = d.brand_id
      AND s.completed_at >= sqlc.arg(range_from)::timestamptz AND s.completed_at < sqlc.arg(range_to)::timestamptz
) sv ON TRUE
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS n FROM orders o
    WHERE o.buyer_org_id = d.id AND o.brand_id = d.brand_id
      AND o.status NOT IN ('draft', 'cancelling', 'cancelled')
      AND o.created_at >= sqlc.arg(range_from)::timestamptz AND o.created_at < sqlc.arg(range_to)::timestamptz
) od ON TRUE
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS n FROM warranties w
    WHERE w.organization_id = d.id AND w.brand_id = d.brand_id
      AND w.start_at >= sqlc.arg(range_from)::timestamptz AND w.start_at < sqlc.arg(range_to)::timestamptz
) wr ON TRUE
WHERE d.brand_id = sqlc.arg(brand_id)
  AND d.type = 'dealer'
  AND d.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR d.id = ANY (sqlc.narg(org_ids)::bigint[]))
ORDER BY services_count DESC, orders_count DESC, lower(d.name), d.id
LIMIT sqlc.arg(row_limit);

-- name: ReportStockAging :many
-- TEC-1229: stock on hand (counted serial units and fixed holdings) of the
-- organizations in reach, per product and warehouse (NULL: held without a
-- location), bucketed by the days since the last ledger movement of each
-- unit / holding (0-30, 30-60, 60-90, 90-180, 180+). unit_price is the
-- brand price list's recommended sale price; the report shows it only
-- with a pricing read permission. TEC-1244: quantity counts closed boxes
-- (and rolls, fixed pieces); an opened box / cut piece of a piece product
-- adds only its meters, so the two measures are never summed together.
WITH stock AS (
    SELECT u.product_id, st.holder_org_id AS org_id, wl.warehouse_id,
           CASE WHEN up.unit_type = 'piece' AND u.remaining_meters IS NOT NULL THEN 0 ELSE 1 END::bigint AS qty,
           COALESCE(u.remaining_meters, 0)::numeric(14,2) AS meters,
           COALESCE(m.created_at, u.created_at) AS last_at
    FROM unit_current_state st
    JOIN units u ON u.id = st.unit_id
    JOIN products up ON up.id = u.product_id
    LEFT JOIN warehouse_locations wl ON wl.id = st.owner_location_id
    LEFT JOIN stock_movements m ON m.id = st.last_movement_id
    WHERE st.status IN ('available', 'placed')
      AND st.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR st.holder_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
    UNION ALL
    SELECT u.product_id, h.holder_org_id AS org_id, wl.warehouse_id,
           h.quantity_on_hand::bigint AS qty, 0::numeric(14,2) AS meters,
           COALESCE(m.created_at, h.updated_at) AS last_at
    FROM fixed_barcode_holdings h
    JOIN units u ON u.id = h.unit_id
    LEFT JOIN warehouse_locations wl ON wl.id = h.owner_location_id
    LEFT JOIN stock_movements m ON m.id = h.last_movement_id
    WHERE h.quantity_on_hand > 0
      AND h.owner_type IN ('warehouse_location', 'organization')
      AND h.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR h.holder_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
), aged AS (
    SELECT s.*, GREATEST(0, EXTRACT(EPOCH FROM (sqlc.arg(as_of)::timestamptz - s.last_at)) / 86400)::int AS age_days
    FROM stock s
)
SELECT p.uuid AS product_uuid, p.sku, p.name AS product_name,
       o.uuid AS organization_uuid, o.name AS organization_name,
       w.uuid AS warehouse_uuid, w.name AS warehouse_name,
       SUM(a.qty)::bigint AS quantity,
       SUM(a.meters)::numeric(14,2)::text AS meters,
       COALESCE(SUM(a.qty) FILTER (WHERE a.age_days < 30), 0)::bigint AS qty_0_30,
       COALESCE(SUM(a.qty) FILTER (WHERE a.age_days >= 30 AND a.age_days < 60), 0)::bigint AS qty_30_60,
       COALESCE(SUM(a.qty) FILTER (WHERE a.age_days >= 60 AND a.age_days < 90), 0)::bigint AS qty_60_90,
       COALESCE(SUM(a.qty) FILTER (WHERE a.age_days >= 90 AND a.age_days < 180), 0)::bigint AS qty_90_180,
       COALESCE(SUM(a.qty) FILTER (WHERE a.age_days >= 180), 0)::bigint AS qty_180_plus,
       MAX(a.age_days)::int AS oldest_days,
       MIN(a.last_at)::timestamptz AS oldest_movement_at,
       COALESCE(pp.recommended_sale_price::numeric(14,4)::text, '')::text AS unit_price,
       b.currency::text AS currency
FROM aged a
JOIN products p ON p.id = a.product_id
JOIN organizations o ON o.id = a.org_id
JOIN brands b ON b.id = p.brand_id
LEFT JOIN warehouses w ON w.id = a.warehouse_id
LEFT JOIN product_prices pp ON pp.product_id = p.id AND pp.currency = b.currency
GROUP BY p.id, p.uuid, p.sku, p.name, o.id, o.uuid, o.name, w.id, w.uuid, w.name,
         pp.recommended_sale_price, b.currency
HAVING MAX(a.age_days) >= sqlc.arg(min_days)::int
ORDER BY MAX(a.age_days) DESC, p.name, p.id, o.id, w.id NULLS FIRST
LIMIT sqlc.arg(row_limit);
