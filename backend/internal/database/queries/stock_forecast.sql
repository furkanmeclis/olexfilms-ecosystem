-- TEC-483 (F5-04a): stock forecast snapshots, thresholds and center network
-- demand. Worker/API layers arrive in F5-04b/c; these queries are the
-- repository contract for daily snapshots and list screens.

-- name: UpsertStockForecastSnapshot :one
WITH demote AS (
    UPDATE stock_forecasts AS sf
    SET is_latest = false
    WHERE sqlc.arg(is_latest)::bool
      AND sf.organization_id = sqlc.arg(organization_id)
      AND sf.product_id = sqlc.arg(product_id)
      AND sf.computed_on <> sqlc.arg(computed_on)::date
    RETURNING 1
), input AS (
    SELECT
        sqlc.arg(organization_id)::bigint AS organization_id,
        sqlc.arg(brand_id)::bigint AS brand_id,
        sqlc.arg(product_id)::bigint AS product_id,
        sqlc.arg(computed_on)::date AS computed_on,
        sqlc.arg(is_latest)::bool AS is_latest,
        sqlc.arg(on_hand_qty)::int AS on_hand_qty,
        sqlc.arg(on_hand_meters)::numeric AS on_hand_meters,
        sqlc.arg(avg_daily_30)::numeric AS avg_daily_30,
        sqlc.arg(avg_daily_90)::numeric AS avg_daily_90,
        sqlc.arg(seasonality_factor)::numeric AS seasonality_factor,
        sqlc.narg(avg_meters_per_vehicle)::numeric AS avg_meters_per_vehicle,
        sqlc.narg(vehicles_left)::numeric AS vehicles_left,
        sqlc.narg(days_left)::numeric AS days_left,
        sqlc.narg(depletion_date)::date AS depletion_date,
        sqlc.arg(data_days)::int AS data_days,
        sqlc.arg(status)::text AS status,
        sqlc.narg(suggested_qty)::int AS suggested_qty,
        sqlc.narg(suggested_meters)::numeric AS suggested_meters
)
INSERT INTO stock_forecasts (
    organization_id, brand_id, product_id, computed_on, is_latest,
    on_hand_qty, on_hand_meters, avg_daily_30, avg_daily_90,
    seasonality_factor, avg_meters_per_vehicle, vehicles_left, days_left,
    depletion_date, data_days, status, suggested_qty, suggested_meters
)
SELECT organization_id, brand_id, product_id, computed_on, is_latest,
       on_hand_qty, on_hand_meters, avg_daily_30, avg_daily_90,
       seasonality_factor, avg_meters_per_vehicle, vehicles_left, days_left,
       depletion_date, data_days, status, suggested_qty, suggested_meters
FROM input
WHERE (SELECT COUNT(*) FROM demote) >= 0
ON CONFLICT (organization_id, product_id, computed_on) DO UPDATE
SET brand_id = EXCLUDED.brand_id,
    is_latest = EXCLUDED.is_latest,
    on_hand_qty = EXCLUDED.on_hand_qty,
    on_hand_meters = EXCLUDED.on_hand_meters,
    avg_daily_30 = EXCLUDED.avg_daily_30,
    avg_daily_90 = EXCLUDED.avg_daily_90,
    seasonality_factor = EXCLUDED.seasonality_factor,
    avg_meters_per_vehicle = EXCLUDED.avg_meters_per_vehicle,
    vehicles_left = EXCLUDED.vehicles_left,
    days_left = EXCLUDED.days_left,
    depletion_date = EXCLUDED.depletion_date,
    data_days = EXCLUDED.data_days,
    status = EXCLUDED.status,
    suggested_qty = EXCLUDED.suggested_qty,
    suggested_meters = EXCLUDED.suggested_meters
RETURNING *;

-- name: DeleteOldStockForecastSnapshots :execrows
DELETE FROM stock_forecasts
WHERE computed_on < sqlc.arg(cutoff_on)::date
  AND NOT is_latest;

-- name: GetLatestStockForecast :one
SELECT * FROM stock_forecasts
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id)
  AND is_latest;

-- name: ListStockForecasts :many
-- List contract: sort=days_left|-days_left|depletion_date|-depletion_date|
-- product_name|-product_name|avg_daily_30|-avg_daily_30|status|-status;
-- default days_left NULLS LAST. product_id is the stable tiebreak.
SELECT sf.*, p.uuid AS product_uuid, p.sku, p.name AS product_name,
       p.unit_type, c.uuid AS category_uuid, c.name AS category_name
FROM stock_forecasts sf
JOIN products p ON p.id = sf.product_id
JOIN product_categories c ON c.id = p.category_id
WHERE sf.organization_id = sqlc.arg(organization_id)
  AND sf.brand_id = sqlc.arg(brand_id)
  AND sf.is_latest
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR sf.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0
    OR p.category_id = ANY (sqlc.narg(category_ids)::bigint[])
  )
  AND (sqlc.narg(days_left_min)::numeric IS NULL OR sf.days_left >= sqlc.narg(days_left_min)::numeric)
  AND (sqlc.narg(days_left_max)::numeric IS NULL OR sf.days_left <= sqlc.narg(days_left_max)::numeric)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'days_left' THEN sf.days_left END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'days_left' THEN sf.days_left END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'depletion_date' THEN sf.depletion_date END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'depletion_date' THEN sf.depletion_date END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'product_name' THEN p.name WHEN 'status' THEN sf.status END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'product_name' THEN p.name WHEN 'status' THEN sf.status END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'avg_daily_30' THEN sf.avg_daily_30 END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'avg_daily_30' THEN sf.avg_daily_30 END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN sf.product_id END DESC,
  sf.product_id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStockForecasts :one
SELECT COUNT(*)::bigint
FROM stock_forecasts sf
JOIN products p ON p.id = sf.product_id
WHERE sf.organization_id = sqlc.arg(organization_id)
  AND sf.brand_id = sqlc.arg(brand_id)
  AND sf.is_latest
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR sf.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0
    OR p.category_id = ANY (sqlc.narg(category_ids)::bigint[])
  )
  AND (sqlc.narg(days_left_min)::numeric IS NULL OR sf.days_left >= sqlc.narg(days_left_min)::numeric)
  AND (sqlc.narg(days_left_max)::numeric IS NULL OR sf.days_left <= sqlc.narg(days_left_max)::numeric)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: ListStockForecastHistory :many
SELECT * FROM stock_forecasts
WHERE organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id)
  AND product_id = sqlc.arg(product_id)
  AND computed_on >= sqlc.arg(from_on)::date
ORDER BY computed_on DESC;

-- name: ListProductConsumptionSeries :many
WITH days AS (
    SELECT generate_series(
        sqlc.arg(from_on)::date,
        sqlc.arg(to_on)::date,
        interval '1 day'
    )::date AS consumed_on
),
daily AS (
    SELECT sm.created_at::date AS consumed_on,
           COALESCE(SUM(GREATEST(-sm.quantity_delta, 0)), 0)::int AS consumed_qty,
           COALESCE(SUM(GREATEST(-sm.meters_delta, 0)), 0)::numeric(14,2) AS consumed_meters
    FROM stock_movements sm
    WHERE sm.organization_id = sqlc.arg(organization_id)
      AND sm.brand_id = sqlc.arg(brand_id)
      AND sm.product_id = sqlc.arg(product_id)
      AND sm.type IN ('consumption', 'partial_consumption', 'sale')
      AND sm.created_at >= sqlc.arg(from_on)::date
      AND sm.created_at < (sqlc.arg(to_on)::date + interval '1 day')
    GROUP BY sm.created_at::date
)
SELECT d.consumed_on,
       COALESCE(daily.consumed_qty, 0)::int AS consumed_qty,
       COALESCE(daily.consumed_meters, 0)::numeric(14,2) AS consumed_meters
FROM days d
LEFT JOIN daily ON daily.consumed_on = d.consumed_on
ORDER BY d.consumed_on ASC;

-- name: UpsertStockForecastThreshold :one
INSERT INTO stock_forecast_thresholds (
    organization_id, brand_id, product_id, warning_days, cover_days
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(product_id),
    sqlc.arg(warning_days), sqlc.arg(cover_days)
)
ON CONFLICT (organization_id, product_id) DO UPDATE
SET brand_id = EXCLUDED.brand_id,
    warning_days = EXCLUDED.warning_days,
    cover_days = EXCLUDED.cover_days
RETURNING *;

-- name: GetStockForecastThreshold :one
SELECT * FROM stock_forecast_thresholds
WHERE organization_id = sqlc.arg(organization_id)
  AND (
    (sqlc.narg(product_id)::bigint IS NULL AND product_id IS NULL)
    OR product_id = sqlc.narg(product_id)::bigint
  );

-- name: ListStockForecastThresholds :many
SELECT t.*, p.uuid AS product_uuid, p.sku, p.name AS product_name
FROM stock_forecast_thresholds t
LEFT JOIN products p ON p.id = t.product_id
WHERE t.organization_id = sqlc.arg(organization_id)
ORDER BY t.product_id NULLS FIRST, p.name ASC, t.id ASC;

-- name: UpsertNetworkDemandForecast :one
INSERT INTO network_demand_forecasts (
    organization_id, brand_id, product_id, forecast_month,
    expected_qty, expected_meters, network_on_hand_qty, network_on_hand_meters,
    open_order_qty, open_order_meters, suggested_production_qty, suggested_production_meters,
    computed_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(forecast_month),
    sqlc.arg(expected_qty), sqlc.arg(expected_meters), sqlc.arg(network_on_hand_qty), sqlc.arg(network_on_hand_meters),
    sqlc.arg(open_order_qty), sqlc.arg(open_order_meters), sqlc.arg(suggested_production_qty),
    sqlc.arg(suggested_production_meters), sqlc.arg(computed_at)
)
ON CONFLICT (brand_id, product_id, forecast_month) DO UPDATE
SET organization_id = EXCLUDED.organization_id,
    expected_qty = EXCLUDED.expected_qty,
    expected_meters = EXCLUDED.expected_meters,
    network_on_hand_qty = EXCLUDED.network_on_hand_qty,
    network_on_hand_meters = EXCLUDED.network_on_hand_meters,
    open_order_qty = EXCLUDED.open_order_qty,
    open_order_meters = EXCLUDED.open_order_meters,
    suggested_production_qty = EXCLUDED.suggested_production_qty,
    suggested_production_meters = EXCLUDED.suggested_production_meters,
    computed_at = EXCLUDED.computed_at
RETURNING *;

-- name: ListNetworkDemandForecasts :many
SELECT ndf.*, p.uuid AS product_uuid, p.sku, p.name AS product_name,
       c.uuid AS category_uuid, c.name AS category_name
FROM network_demand_forecasts ndf
JOIN products p ON p.id = ndf.product_id
JOIN product_categories c ON c.id = p.category_id
WHERE ndf.brand_id = sqlc.arg(brand_id)
  AND ndf.forecast_month >= sqlc.arg(from_month)::date
  AND ndf.forecast_month < sqlc.arg(before_month)::date
  AND (
    COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0
    OR p.category_id = ANY (sqlc.narg(category_ids)::bigint[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY ndf.forecast_month ASC, p.name ASC, ndf.product_id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountNetworkDemandForecasts :one
SELECT COUNT(*)::bigint
FROM network_demand_forecasts ndf
JOIN products p ON p.id = ndf.product_id
WHERE ndf.brand_id = sqlc.arg(brand_id)
  AND ndf.forecast_month >= sqlc.arg(from_month)::date
  AND ndf.forecast_month < sqlc.arg(before_month)::date
  AND (
    COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0
    OR p.category_id = ANY (sqlc.narg(category_ids)::bigint[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: ListStockForecastOrganizations :many
SELECT *
FROM organizations
WHERE deleted_at IS NULL
  AND status = 'active'
  AND type IN ('center', 'distributor', 'dealer')
  AND (sqlc.narg(organization_id)::bigint IS NULL OR id = sqlc.narg(organization_id)::bigint)
ORDER BY brand_id, id;

-- name: ListStockForecastProductsForOrg :many
SELECT DISTINCT p.*
FROM products p
WHERE p.brand_id = sqlc.arg(brand_id)
  AND p.active
  AND (
    EXISTS (
      SELECT 1 FROM organization_product_stocks ops
      WHERE ops.organization_id = sqlc.arg(organization_id)
        AND ops.product_id = p.id
        AND (ops.quantity > 0 OR ops.meters > 0)
    )
    OR EXISTS (
      SELECT 1 FROM stock_movements sm
      WHERE sm.organization_id = sqlc.arg(organization_id)
        AND sm.brand_id = sqlc.arg(brand_id)
        AND sm.product_id = p.id
    )
    OR EXISTS (
      SELECT 1
      FROM orders o
      JOIN order_items oi ON oi.order_id = o.id
      WHERE o.buyer_org_id = sqlc.arg(organization_id)
        AND o.brand_id = sqlc.arg(brand_id)
        AND oi.product_id = p.id
        AND o.status IN ('submitted', 'approved', 'preparing', 'ready', 'processing', 'shipped', 'delivered')
    )
  )
ORDER BY p.id;

-- name: GetStockForecastOnHand :one
SELECT COALESCE(quantity, 0)::int AS quantity,
       COALESCE(meters, 0)::numeric(14,2) AS meters
FROM organization_product_stocks
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id);

-- name: GetStockForecastFirstMovementDate :one
SELECT MIN(created_at)::date
FROM stock_movements
WHERE organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id);

-- name: GetStockForecastConsumptionTotals :one
SELECT
  COALESCE(SUM(
    CASE
      WHEN sm.type IN ('consumption', 'partial_consumption', 'sale')
        THEN GREATEST(-sm.quantity_delta, 0)
      WHEN sqlc.arg(include_order_out)::bool AND sm.type = 'order_out'
        THEN GREATEST(-sm.quantity_delta, 0)
      WHEN sm.type IN ('return', 'void')
        THEN -GREATEST(sm.quantity_delta, 0)
      WHEN sqlc.arg(include_order_out)::bool AND sm.type = 'order_cancel_restore'
        THEN -GREATEST(sm.quantity_delta, 0)
      ELSE 0
    END
  ), 0)::numeric AS qty,
  COALESCE(SUM(
    CASE
      WHEN sm.type IN ('consumption', 'partial_consumption', 'sale')
        THEN GREATEST(-sm.meters_delta, 0)
      WHEN sqlc.arg(include_order_out)::bool AND sm.type = 'order_out'
        THEN GREATEST(-sm.meters_delta, 0)
      WHEN sm.type IN ('return', 'void')
        THEN -GREATEST(sm.meters_delta, 0)
      WHEN sqlc.arg(include_order_out)::bool AND sm.type = 'order_cancel_restore'
        THEN -GREATEST(sm.meters_delta, 0)
      ELSE 0
    END
  ), 0)::numeric AS meters
FROM stock_movements sm
WHERE sm.brand_id = sqlc.arg(brand_id)
  AND sm.product_id = sqlc.arg(product_id)
  AND (
    sm.organization_id = sqlc.arg(organization_id)
    OR (
      sqlc.arg(include_order_out)::bool
      AND sm.type = 'order_cancel_restore'
      AND EXISTS (
        SELECT 1 FROM stock_movements out_sm
        WHERE out_sm.organization_id = sqlc.arg(organization_id)
          AND out_sm.brand_id = sm.brand_id
          AND out_sm.product_id = sm.product_id
          AND out_sm.unit_id = sm.unit_id
          AND out_sm.type = 'order_out'
          AND out_sm.reference_type IS NOT DISTINCT FROM sm.reference_type
          AND out_sm.reference_id IS NOT DISTINCT FROM sm.reference_id
      )
    )
  )
  AND sm.created_at >= sqlc.arg(from_at)::timestamptz
  AND sm.created_at < sqlc.arg(to_at)::timestamptz;

-- name: GetStockForecastSeasonalityTotals :one
SELECT
  COALESCE(SUM(CASE WHEN month_match THEN consumption ELSE 0 END), 0)::numeric AS same_month,
  COALESCE(SUM(consumption), 0)::numeric AS year_total
FROM (
  SELECT date_trunc('month', sm.created_at)::date = sqlc.arg(same_month)::date AS month_match,
         CASE
           WHEN sm.type IN ('consumption', 'partial_consumption', 'sale')
             THEN GREATEST(-sm.quantity_delta, 0)::numeric + GREATEST(-sm.meters_delta, 0)
           WHEN sqlc.arg(include_order_out)::bool AND sm.type = 'order_out'
             THEN GREATEST(-sm.quantity_delta, 0)::numeric + GREATEST(-sm.meters_delta, 0)
           WHEN sm.type IN ('return', 'void')
             THEN -(GREATEST(sm.quantity_delta, 0)::numeric + GREATEST(sm.meters_delta, 0))
           WHEN sqlc.arg(include_order_out)::bool AND sm.type = 'order_cancel_restore'
             THEN -(GREATEST(sm.quantity_delta, 0)::numeric + GREATEST(sm.meters_delta, 0))
           ELSE 0
         END AS consumption
  FROM stock_movements sm
  WHERE sm.brand_id = sqlc.arg(brand_id)
    AND sm.product_id = sqlc.arg(product_id)
    AND (
      sm.organization_id = sqlc.arg(organization_id)
      OR (
        sqlc.arg(include_order_out)::bool
        AND sm.type = 'order_cancel_restore'
        AND EXISTS (
          SELECT 1 FROM stock_movements out_sm
          WHERE out_sm.organization_id = sqlc.arg(organization_id)
            AND out_sm.brand_id = sm.brand_id
            AND out_sm.product_id = sm.product_id
            AND out_sm.unit_id = sm.unit_id
            AND out_sm.type = 'order_out'
            AND out_sm.reference_type IS NOT DISTINCT FROM sm.reference_type
            AND out_sm.reference_id IS NOT DISTINCT FROM sm.reference_id
        )
      )
    )
    AND sm.created_at >= sqlc.arg(from_at)::timestamptz
    AND sm.created_at < sqlc.arg(to_at)::timestamptz
) s;

-- name: GetStockForecastPartialMetersAndServices :one
SELECT
  COALESCE((
    SELECT SUM(GREATEST(-sm.meters_delta, 0))
    FROM stock_movements sm
    WHERE sm.organization_id = sqlc.arg(organization_id)
      AND sm.brand_id = sqlc.arg(brand_id)
      AND sm.product_id = sqlc.arg(product_id)
      AND sm.type = 'partial_consumption'
      AND sm.created_at >= sqlc.arg(from_at)::timestamptz
      AND sm.created_at < sqlc.arg(to_at)::timestamptz
  ), 0)::numeric AS meters,
  COALESCE((
    SELECT COUNT(*)::bigint
    FROM services s
    WHERE s.organization_id = sqlc.arg(organization_id)
      AND s.brand_id = sqlc.arg(brand_id)
      AND s.status = 'completed'
      AND s.completed_at >= sqlc.arg(from_at)::timestamptz
      AND s.completed_at < sqlc.arg(to_at)::timestamptz
  ), 0)::bigint AS services;

-- name: GetStockForecastOpenIncomingOrders :one
SELECT
  COALESCE(SUM(oi.quantity), 0)::int AS qty,
  COALESCE(SUM(oi.meters), 0)::numeric(14,2) AS meters
FROM orders o
JOIN order_items oi ON oi.order_id = o.id
WHERE o.buyer_org_id = sqlc.arg(organization_id)
  AND o.brand_id = sqlc.arg(brand_id)
  AND oi.product_id = sqlc.arg(product_id)
  AND o.status IN ('submitted', 'approved', 'preparing', 'ready', 'processing', 'shipped', 'delivered');

-- name: ListStockForecastNetworkInputs :many
WITH months AS (
  SELECT date_trunc('month', sqlc.arg(computed_on)::date + (n || ' months')::interval)::date AS forecast_month,
         EXTRACT(day FROM (
           date_trunc('month', sqlc.arg(computed_on)::date + ((n + 1) || ' months')::interval)
           - date_trunc('month', sqlc.arg(computed_on)::date + (n || ' months')::interval)
         ))::numeric AS days_in_month
  FROM generate_series(1, 3) AS n
),
latest AS (
  SELECT sf.organization_id, sf.brand_id, sf.product_id, p.unit_type,
         (sf.avg_daily_30 * 0.6 + sf.avg_daily_90 * 0.4) AS base_daily,
         sf.data_days, o.type AS organization_type
  FROM stock_forecasts sf
  JOIN products p ON p.id = sf.product_id AND p.brand_id = sf.brand_id
  JOIN organizations o ON o.id = sf.organization_id
  WHERE sf.brand_id = sqlc.arg(brand_id)
    AND sf.is_latest
    AND sf.status <> 'insufficient_data'
),
demand AS (
  SELECT l.product_id, m.forecast_month,
         CEIL(COALESCE(SUM(
           CASE WHEN l.unit_type = 'roll_meter' THEN 0
                ELSE l.base_daily * f.factor * m.days_in_month
           END
         ), 0))::int AS expected_qty,
         COALESCE(SUM(
           CASE WHEN l.unit_type = 'roll_meter' THEN l.base_daily * f.factor * m.days_in_month
                ELSE 0
           END
         ), 0)::numeric(14,2) AS expected_meters
  FROM latest l
  CROSS JOIN months m
  CROSS JOIN LATERAL (
    SELECT CASE
      WHEN l.data_days >= 365 AND totals.year_total > 0
        THEN LEAST(2.0, GREATEST(0.5, totals.same_month / (totals.year_total / 12)))
      ELSE 1.0
    END AS factor
    FROM (
      SELECT
        COALESCE(SUM(CASE WHEN date_trunc('month', sm.created_at)::date = (m.forecast_month - interval '1 year')::date
          THEN movement.consumption ELSE 0 END), 0)::numeric AS same_month,
        COALESCE(SUM(movement.consumption), 0)::numeric AS year_total
      FROM stock_movements sm
      CROSS JOIN LATERAL (
        SELECT CASE
          WHEN sm.type IN ('consumption', 'partial_consumption', 'sale')
            THEN GREATEST(-sm.quantity_delta, 0)::numeric + GREATEST(-sm.meters_delta, 0)
          WHEN l.organization_type IN ('center', 'distributor') AND sm.type = 'order_out'
            THEN GREATEST(-sm.quantity_delta, 0)::numeric + GREATEST(-sm.meters_delta, 0)
          WHEN sm.type IN ('return', 'void')
            THEN -(GREATEST(sm.quantity_delta, 0)::numeric + GREATEST(sm.meters_delta, 0))
          WHEN l.organization_type IN ('center', 'distributor') AND sm.type = 'order_cancel_restore'
            THEN -(GREATEST(sm.quantity_delta, 0)::numeric + GREATEST(sm.meters_delta, 0))
          ELSE 0
        END AS consumption
      ) movement
      WHERE sm.organization_id = l.organization_id
        AND sm.brand_id = l.brand_id
        AND sm.product_id = l.product_id
        AND sm.created_at >= date_trunc('year', m.forecast_month - interval '1 year')
        AND sm.created_at < date_trunc('year', m.forecast_month - interval '1 year') + interval '1 year'
    ) totals
  ) f
  GROUP BY l.product_id, m.forecast_month
),
products_in_scope AS (
  SELECT p.id AS product_id
  FROM products p
  WHERE p.brand_id = sqlc.arg(brand_id)
    AND p.active
    AND (
      EXISTS (SELECT 1 FROM latest l WHERE l.product_id = p.id)
      OR EXISTS (
        SELECT 1 FROM organization_product_stocks ops
        WHERE ops.organization_id = sqlc.arg(organization_id)
          AND ops.product_id = p.id
          AND (ops.quantity > 0 OR ops.meters > 0)
      )
      OR EXISTS (
        SELECT 1
        FROM orders o
        JOIN order_items oi ON oi.order_id = o.id
        WHERE o.buyer_org_id = sqlc.arg(organization_id)
          AND o.brand_id = sqlc.arg(brand_id)
          AND oi.product_id = p.id
          AND o.status IN ('submitted', 'approved', 'preparing', 'ready', 'processing', 'shipped', 'delivered')
      )
    )
)
SELECT p.product_id, m.forecast_month,
       COALESCE(d.expected_qty, 0)::int AS expected_qty,
       COALESCE(d.expected_meters, 0)::numeric(14,2) AS expected_meters,
       COALESCE(ops.quantity, 0)::int AS network_on_hand_qty,
       COALESCE(ops.meters, 0)::numeric(14,2) AS network_on_hand_meters,
       COALESCE(open_orders.qty, 0)::int AS open_order_qty,
       COALESCE(open_orders.meters, 0)::numeric(14,2) AS open_order_meters
FROM products_in_scope p
CROSS JOIN months m
LEFT JOIN demand d ON d.product_id = p.product_id AND d.forecast_month = m.forecast_month
LEFT JOIN organization_product_stocks ops
  ON ops.organization_id = sqlc.arg(organization_id)
 AND ops.product_id = p.product_id
LEFT JOIN LATERAL (
  SELECT COALESCE(SUM(oi.quantity), 0)::int AS qty,
         COALESCE(SUM(oi.meters), 0)::numeric(14,2) AS meters
  FROM orders o
  JOIN order_items oi ON oi.order_id = o.id
  WHERE o.buyer_org_id = sqlc.arg(organization_id)
    AND o.brand_id = sqlc.arg(brand_id)
    AND oi.product_id = p.product_id
    AND o.status IN ('submitted', 'approved', 'preparing', 'ready', 'processing', 'shipped', 'delivered')
) open_orders ON true
ORDER BY p.product_id, m.forecast_month;

-- name: ListStockForecastNotifyUserIDs :many
SELECT DISTINCT om.user_id
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
LEFT JOIN organization_member_roles mr ON mr.member_id = om.id
LEFT JOIN roles r ON r.id = mr.role_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (om.role = 'owner' OR r.slug LIKE '%warehouse%')
ORDER BY om.user_id;
