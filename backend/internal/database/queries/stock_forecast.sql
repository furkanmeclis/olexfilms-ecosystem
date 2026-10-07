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
