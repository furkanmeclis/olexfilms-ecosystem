-- TEC-490 (F5-05a): performance and targets. Authorization scope is
-- resolved by the usecase and arrives as organization id lists (NULL = the
-- whole brand). Metric keys: internal/modules/performance/model.

-- Monthly metrics ---------------------------------------------------------------

-- name: UpsertPerformanceMetric :one
-- Idempotent write of the metric worker: one row per org x month x metric.
INSERT INTO performance_metrics_monthly (
    organization_id, brand_id, period, scope, metric, value, numerator, denominator, currency, computed_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(period), sqlc.arg(scope), sqlc.arg(metric),
    sqlc.arg(value), sqlc.narg(numerator), sqlc.narg(denominator), sqlc.narg(currency), NOW()
)
ON CONFLICT (organization_id, period, scope, metric) DO UPDATE
SET value = EXCLUDED.value,
    numerator = EXCLUDED.numerator,
    denominator = EXCLUDED.denominator,
    currency = EXCLUDED.currency,
    computed_at = EXCLUDED.computed_at
RETURNING *;

-- name: DeletePerformanceMetricsForScope :execrows
DELETE FROM performance_metrics_monthly
WHERE organization_id = sqlc.arg(organization_id)
  AND period = sqlc.arg(period)::text
  AND scope = sqlc.arg(scope)::text;

-- name: ListPerformanceMetrics :many
-- Metrics of the organizations over a closed period range (YYYY-MM).
SELECT m.*
FROM performance_metrics_monthly m
WHERE m.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND m.period >= sqlc.arg(period_from)::text
  AND m.period <= sqlc.arg(period_to)::text
  AND (sqlc.narg(scope)::text IS NULL OR m.scope = sqlc.narg(scope)::text)
  AND (COALESCE(cardinality(sqlc.narg(metrics)::text[]), 0) = 0 OR m.metric = ANY (sqlc.narg(metrics)::text[]))
ORDER BY m.organization_id, m.period, m.scope, m.metric;

-- name: ListPerformanceRanking :many
-- Ranking list of distributors and dealers for one month, one column per
-- metric (NULL = not computed). Sort: docs/list-contract.md, keys from
-- performance/repository.RankingSort (metric keys | name); metric columns
-- sort NULLS LAST in both directions; id tiebreak.
WITH m AS (
    SELECT pm.organization_id,
           MAX(pm.value) FILTER (WHERE pm.metric = 'services_count') AS services_count,
           MAX(pm.value) FILTER (WHERE pm.metric = 'warranty_start_rate') AS warranty_start_rate,
           MAX(pm.value) FILTER (WHERE pm.metric = 'measurement_rate') AS measurement_rate,
           MAX(pm.value) FILTER (WHERE pm.metric = 'review_avg') AS review_avg,
           MAX(pm.value) FILTER (WHERE pm.metric = 'stock_turnover') AS stock_turnover,
           MAX(pm.value) FILTER (WHERE pm.metric = 'contract_days_left') AS contract_days_left,
           MAX(pm.value) FILTER (WHERE pm.metric = 'cari_overdue_amount') AS cari_overdue_amount,
           MAX(pm.value) FILTER (WHERE pm.metric = 'cari_overdue_days') AS cari_overdue_days,
           MAX(pm.value) FILTER (WHERE pm.metric = 'certificate_coverage') AS certificate_coverage,
           MAX(pm.value) FILTER (WHERE pm.metric = 'lead_conversion_rate') AS lead_conversion_rate,
           MAX(pm.value) FILTER (WHERE pm.metric = 'waste_ratio') AS waste_ratio,
           MAX(pm.value) FILTER (WHERE pm.metric = 'order_volume') AS order_volume,
           MAX(pm.computed_at)::timestamptz AS computed_at
    FROM performance_metrics_monthly pm
    WHERE pm.brand_id = sqlc.arg(brand_id)
      AND pm.period = sqlc.arg(period)::text
      AND pm.scope = sqlc.arg(scope)::text
    GROUP BY pm.organization_id
)
SELECT o.id AS organization_id,
       o.uuid AS organization_uuid,
       o.name,
       o.type,
       o.parent_id,
       o.province_id,
       o.currency,
       m.services_count::numeric AS services_count,
       m.warranty_start_rate::numeric AS warranty_start_rate,
       m.measurement_rate::numeric AS measurement_rate,
       m.review_avg::numeric AS review_avg,
       m.stock_turnover::numeric AS stock_turnover,
       m.contract_days_left::numeric AS contract_days_left,
       m.cari_overdue_amount::numeric AS cari_overdue_amount,
       m.cari_overdue_days::numeric AS cari_overdue_days,
       m.certificate_coverage::numeric AS certificate_coverage,
       m.lead_conversion_rate::numeric AS lead_conversion_rate,
       m.waste_ratio::numeric AS waste_ratio,
       m.order_volume::numeric AS order_volume,
       m.computed_at AS computed_at,
       COUNT(*) OVER()::bigint AS total_count
FROM organizations o
LEFT JOIN m ON m.organization_id = o.id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND o.type IN ('distributor', 'dealer')
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(org_types)::text[]), 0) = 0 OR o.type = ANY (sqlc.narg(org_types)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(distributor_ids)::bigint[]), 0) = 0
       OR o.id = ANY (sqlc.narg(distributor_ids)::bigint[])
       OR o.parent_id = ANY (sqlc.narg(distributor_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(province_ids)::bigint[]), 0) = 0
       OR o.province_id = ANY (sqlc.narg(province_ids)::bigint[]))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN o.name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN o.name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'services_count' THEN m.services_count
      WHEN 'warranty_start_rate' THEN m.warranty_start_rate
      WHEN 'measurement_rate' THEN m.measurement_rate
      WHEN 'review_avg' THEN m.review_avg
      WHEN 'stock_turnover' THEN m.stock_turnover
      WHEN 'contract_days_left' THEN m.contract_days_left
      WHEN 'cari_overdue_amount' THEN m.cari_overdue_amount
      WHEN 'cari_overdue_days' THEN m.cari_overdue_days
      WHEN 'certificate_coverage' THEN m.certificate_coverage
      WHEN 'lead_conversion_rate' THEN m.lead_conversion_rate
      WHEN 'waste_ratio' THEN m.waste_ratio
      WHEN 'order_volume' THEN m.order_volume
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'services_count' THEN m.services_count
      WHEN 'warranty_start_rate' THEN m.warranty_start_rate
      WHEN 'measurement_rate' THEN m.measurement_rate
      WHEN 'review_avg' THEN m.review_avg
      WHEN 'stock_turnover' THEN m.stock_turnover
      WHEN 'contract_days_left' THEN m.contract_days_left
      WHEN 'cari_overdue_amount' THEN m.cari_overdue_amount
      WHEN 'cari_overdue_days' THEN m.cari_overdue_days
      WHEN 'certificate_coverage' THEN m.certificate_coverage
      WHEN 'lead_conversion_rate' THEN m.lead_conversion_rate
      WHEN 'waste_ratio' THEN m.waste_ratio
      WHEN 'order_volume' THEN m.order_volume
    END
  END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN o.id END DESC,
  o.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Worker source readers ----------------------------------------------------------

-- name: ListPerformanceOrganizations :many
SELECT *
FROM organizations
WHERE deleted_at IS NULL
  AND status = 'active'
  AND type IN ('center', 'distributor', 'dealer')
  AND (sqlc.narg(organization_id)::bigint IS NULL OR id = sqlc.narg(organization_id)::bigint)
ORDER BY brand_id, id;

-- name: ListPerformanceSubtreeOrgIDs :many
WITH RECURSIVE tree AS (
    SELECT o.id
    FROM organizations o
    WHERE o.id = sqlc.arg(root_org_id)::bigint AND o.deleted_at IS NULL
    UNION ALL
    SELECT c.id
    FROM organizations c
    JOIN tree t ON c.parent_id = t.id
    WHERE c.deleted_at IS NULL
)
SELECT id FROM tree ORDER BY id;

-- name: ComputePerformanceMetrics :one
WITH completed_services AS (
    SELECT s.*
    FROM services s
    WHERE s.brand_id = sqlc.arg(brand_id)
      AND s.organization_id = ANY(sqlc.arg(org_ids)::bigint[])
      AND s.status = 'completed'
      AND COALESCE(s.completed_at, s.created_at) >= sqlc.arg(period_from)::timestamptz
      AND COALESCE(s.completed_at, s.created_at) < sqlc.arg(period_to)::timestamptz
),
service_staff AS (
    SELECT DISTINCT COALESCE(s.performed_by_user_id, s.completed_by_user_id, s.created_by_user_id) AS user_id
    FROM completed_services s
    WHERE COALESCE(s.performed_by_user_id, s.completed_by_user_id, s.created_by_user_id) IS NOT NULL
),
service_stats AS (
    SELECT COUNT(*)::numeric(18,4) AS services_count,
           COUNT(*) FILTER (
               WHERE EXISTS (SELECT 1 FROM warranties w WHERE w.service_id = completed_services.id)
           )::numeric(18,4) AS warranty_started,
           COUNT(*) FILTER (WHERE has_measurement)::numeric(18,4) AS measurements
    FROM completed_services
),
review_stats AS (
    SELECT AVG(sr.platform_rating)::numeric(18,4) AS review_avg,
           COUNT(sr.id)::numeric(18,4) AS review_count
    FROM service_reviews sr
    JOIN completed_services cs ON cs.id = sr.service_id
),
valid_certified_staff AS (
    SELECT DISTINCT c.user_id
    FROM certificates c
    JOIN service_staff ss ON ss.user_id = c.user_id
    WHERE c.brand_id = sqlc.arg(brand_id)
      AND c.status = 'valid'
      AND c.issued_at < sqlc.arg(period_to)::timestamptz
      AND (c.expires_at IS NULL OR c.expires_at >= sqlc.arg(period_from)::timestamptz)
),
certificate_stats AS (
    SELECT COUNT(DISTINCT service_staff.user_id)::numeric(18,4) AS service_staff_count,
           COUNT(DISTINCT valid_certified_staff.user_id)::numeric(18,4) AS certified_staff_count
    FROM service_staff
    LEFT JOIN valid_certified_staff ON valid_certified_staff.user_id = service_staff.user_id
),
stock_consumption AS (
    SELECT COALESCE(SUM(ABS(sm.quantity_delta)), 0)::numeric AS qty,
           COALESCE(SUM(ABS(sm.meters_delta)), 0)::numeric AS meters
    FROM stock_movements sm
    WHERE sm.brand_id = sqlc.arg(brand_id)
      AND sm.organization_id = ANY(sqlc.arg(org_ids)::bigint[])
      AND sm.created_at >= sqlc.arg(period_from)::timestamptz
      AND sm.created_at < sqlc.arg(period_to)::timestamptz
      AND sm.type IN ('consumption', 'partial_consumption')
),
stock_on_hand AS (
    SELECT COALESCE(SUM(ops.quantity), 0)::numeric AS qty,
           COALESCE(SUM(ops.meters), 0)::numeric AS meters
    FROM organization_product_stocks ops
    WHERE ops.organization_id = ANY(sqlc.arg(org_ids)::bigint[])
),
cari_balances AS (
    SELECT c.id,
           COALESCE(SUM(CASE e.direction
               WHEN 'income' THEN e.amount
               WHEN 'charge' THEN e.amount
               WHEN 'payment' THEN e.amount
               WHEN 'expense' THEN -e.amount
               WHEN 'collection' THEN -e.amount
           END), 0)::numeric AS balance
    FROM cari_accounts c
    LEFT JOIN finance_entries e ON e.cari_id = c.id AND e.created_at < sqlc.arg(as_of)::timestamptz
    WHERE c.brand_id = sqlc.arg(brand_id)
      AND c.organization_id = ANY(sqlc.arg(org_ids)::bigint[])
    GROUP BY c.id
),
cari_stats AS (
    SELECT COALESCE(SUM(balance) FILTER (WHERE balance > 0), 0)::numeric(18,4) AS overdue_amount
    FROM cari_balances
),
cari_open_entries AS (
    SELECT c.id AS cari_id, e.created_at,
           CASE e.direction
               WHEN 'income' THEN e.amount
               WHEN 'charge' THEN e.amount
               WHEN 'payment' THEN e.amount
               WHEN 'expense' THEN -e.amount
               WHEN 'collection' THEN -e.amount
           END::numeric AS signed_amount,
           b.balance
    FROM cari_balances b
    JOIN cari_accounts c ON c.id = b.id
    JOIN finance_entries e ON e.cari_id = c.id AND e.created_at < sqlc.arg(as_of)::timestamptz
    WHERE b.balance > 0
),
cari_oldest AS (
    SELECT MIN(created_at) AS oldest_at
    FROM cari_open_entries
    WHERE signed_amount > 0
),
closed_leads AS (
    SELECT l.*
    FROM leads l
    WHERE l.brand_id = sqlc.arg(brand_id)
      AND l.organization_id = ANY(sqlc.arg(org_ids)::bigint[])
      AND l.deleted_at IS NULL
      AND l.status IN ('won', 'lost')
      AND l.updated_at >= sqlc.arg(period_from)::timestamptz
      AND l.updated_at < sqlc.arg(period_to)::timestamptz
),
lead_stats AS (
    SELECT COUNT(*)::numeric(18,4) AS closed_leads,
           COUNT(*) FILTER (WHERE status = 'won')::numeric(18,4) AS won_leads
    FROM closed_leads
),
waste AS (
    SELECT AVG(f.waste_ratio)::numeric(18,4) AS avg_waste_ratio
    FROM efficiency_facts f
    WHERE f.brand_id = sqlc.arg(brand_id)
      AND f.dealer_org_id = ANY(sqlc.arg(org_ids)::bigint[])
      AND f.service_date >= sqlc.arg(period_from)::date
      AND f.service_date < sqlc.arg(period_to)::date
),
orders_in AS (
    SELECT COALESCE(SUM(o.total), 0)::numeric(18,4) AS total
    FROM orders o
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.buyer_org_id = ANY(sqlc.arg(org_ids)::bigint[])
      AND o.status IN ('approved', 'preparing', 'ready', 'processing', 'shipped', 'delivered', 'received')
      AND COALESCE(o.approved_at, o.created_at) >= sqlc.arg(period_from)::timestamptz
      AND COALESCE(o.approved_at, o.created_at) < sqlc.arg(period_to)::timestamptz
)
SELECT service_stats.services_count,
       service_stats.warranty_started,
       service_stats.measurements,
       review_stats.review_avg,
       review_stats.review_count,
       stock_consumption.qty::numeric(18,4) AS stock_consumed_qty,
       stock_consumption.meters::numeric(18,4) AS stock_consumed_meters,
       stock_on_hand.qty::numeric(18,4) AS stock_on_hand_qty,
       stock_on_hand.meters::numeric(18,4) AS stock_on_hand_meters,
       cari_stats.overdue_amount AS cari_overdue_amount,
       COALESCE(EXTRACT(DAY FROM (sqlc.arg(as_of)::timestamptz - cari_oldest.oldest_at)), 0)::numeric(18,4) AS cari_overdue_days,
       certificate_stats.service_staff_count,
       certificate_stats.certified_staff_count,
       lead_stats.closed_leads,
       lead_stats.won_leads,
       waste.avg_waste_ratio::numeric(18,4) AS waste_ratio,
       orders_in.total::numeric(18,4) AS order_volume
FROM service_stats
CROSS JOIN review_stats
CROSS JOIN stock_consumption
CROSS JOIN stock_on_hand
CROSS JOIN cari_stats
CROSS JOIN cari_oldest
CROSS JOIN certificate_stats
CROSS JOIN lead_stats
CROSS JOIN waste
CROSS JOIN orders_in;

-- Targets ---------------------------------------------------------------------------

-- name: CreatePerformanceTarget :one
INSERT INTO performance_targets (
    organization_id, brand_id, target_org_id, metric, period_kind, period_start,
    value, currency, contract_ref, note, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(target_org_id), sqlc.arg(metric),
    sqlc.arg(period_kind), sqlc.arg(period_start), sqlc.arg(value), sqlc.narg(currency),
    sqlc.narg(contract_ref), sqlc.narg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetPerformanceTarget :one
SELECT * FROM performance_targets
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdatePerformanceTarget :one
UPDATE performance_targets
SET value = sqlc.arg(value),
    currency = sqlc.narg(currency),
    contract_ref = sqlc.narg(contract_ref),
    note = sqlc.narg(note)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeletePerformanceTarget :execrows
DELETE FROM performance_targets
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListPerformanceTargets :many
-- Targets with their achievement: the target metric summed over the months
-- of the target period (same currency for order volume). Sort keys from
-- performance/repository.TargetSort; id tiebreak.
SELECT t.*,
       o.name AS target_name,
       o.type AS target_type,
       a.actual::numeric AS actual,
       CASE WHEN a.actual IS NULL THEN NULL
            ELSE ROUND(a.actual / t.value * 100, 2)
       END::numeric AS achievement_pct,
       COUNT(*) OVER()::bigint AS total_count
FROM performance_targets t
JOIN organizations o ON o.id = t.target_org_id
LEFT JOIN LATERAL (
    SELECT SUM(pm.value) AS actual
    FROM performance_metrics_monthly pm
    WHERE pm.organization_id = t.target_org_id
      AND pm.metric = t.metric
      AND pm.currency IS NOT DISTINCT FROM t.currency
      AND pm.period >= to_char(t.period_start, 'YYYY-MM')
      AND pm.period < to_char(t.period_end, 'YYYY-MM')
) a ON true
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(owner_org_ids)::bigint[] IS NULL OR t.organization_id = ANY (sqlc.narg(owner_org_ids)::bigint[]))
  AND (sqlc.narg(target_org_ids)::bigint[] IS NULL OR t.target_org_id = ANY (sqlc.narg(target_org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(metrics)::text[]), 0) = 0 OR t.metric = ANY (sqlc.narg(metrics)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(period_kinds)::text[]), 0) = 0 OR t.period_kind = ANY (sqlc.narg(period_kinds)::text[]))
  AND (sqlc.narg(period_from)::date IS NULL OR t.period_end > sqlc.narg(period_from)::date)
  AND (sqlc.narg(period_before)::date IS NULL OR t.period_start < sqlc.narg(period_before)::date)
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'target_name' THEN o.name
      WHEN 'metric' THEN t.metric
      WHEN 'period_kind' THEN t.period_kind
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'target_name' THEN o.name
      WHEN 'metric' THEN t.metric
      WHEN 'period_kind' THEN t.period_kind
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'period_start' THEN t.period_start END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'period_start' THEN t.period_start END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN t.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN t.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'value' THEN t.value
      WHEN 'achievement_pct' THEN a.actual / t.value
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'value' THEN t.value
      WHEN 'achievement_pct' THEN a.actual / t.value
    END
  END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Staff targets ---------------------------------------------------------------------

-- Region map ------------------------------------------------------------------------

-- name: ListPerformanceMapRegions :many
WITH visible_territories AS (
    SELECT t.*
    FROM territories t
    WHERE t.brand_id = sqlc.arg(brand_id)
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR (sqlc.arg(actor_type)::text = 'distributor' AND t.organization_id = sqlc.arg(actor_org_id)::bigint)
      )
),
visible_dealers AS (
    SELECT o.*
    FROM organizations o
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.deleted_at IS NULL
      AND o.type = 'dealer'
      AND (sqlc.narg(country_iso2)::text IS NULL OR EXISTS (
        SELECT 1 FROM countries c WHERE c.id = o.country_id AND c.iso2 = sqlc.narg(country_iso2)::text
      ))
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR (sqlc.arg(actor_type)::text = 'dealer' AND o.id = sqlc.arg(actor_org_id)::bigint)
        OR (
          sqlc.arg(actor_type)::text = 'distributor'
          AND EXISTS (
            SELECT 1
            FROM visible_territories t
            WHERE t.country_id = o.country_id
              AND (t.province_id IS NULL OR t.province_id = o.province_id)
              AND (t.district_id IS NULL OR t.district_id = o.district_id)
          )
        )
      )
),
areas AS (
    SELECT 'country'::text AS level,
           c.id AS area_id,
           c.iso2::text AS code,
           c.name_tr AS name,
           c.id AS country_id,
           NULL::bigint AS province_id,
           NULL::bigint AS district_id,
           NULL::numeric AS area_latitude,
           NULL::numeric AS area_longitude
    FROM countries c
    WHERE sqlc.arg(level)::text = 'country'
      AND c.is_active
      AND (sqlc.narg(country_iso2)::text IS NULL OR c.iso2 = sqlc.narg(country_iso2)::text)
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR EXISTS (SELECT 1 FROM visible_territories t WHERE t.country_id = c.id)
        OR EXISTS (SELECT 1 FROM visible_dealers d WHERE d.country_id = c.id)
      )
    UNION ALL
    SELECT 'province'::text AS level,
           p.id AS area_id,
           p.code::text AS code,
           p.name,
           p.country_id,
           p.id AS province_id,
           NULL::bigint AS district_id,
           p.latitude AS area_latitude,
           p.longitude AS area_longitude
    FROM provinces p
    JOIN countries c ON c.id = p.country_id
    WHERE sqlc.arg(level)::text = 'province'
      AND (sqlc.narg(country_iso2)::text IS NULL OR c.iso2 = sqlc.narg(country_iso2)::text)
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR EXISTS (
          SELECT 1 FROM visible_territories t
          WHERE t.country_id = p.country_id AND (t.province_id IS NULL OR t.province_id = p.id)
        )
        OR EXISTS (SELECT 1 FROM visible_dealers d WHERE d.province_id = p.id)
      )
    UNION ALL
    SELECT 'district'::text AS level,
           d.id AS area_id,
           d.code::text AS code,
           d.name,
           p.country_id,
           p.id AS province_id,
           d.id AS district_id,
           d.latitude AS area_latitude,
           d.longitude AS area_longitude
    FROM districts d
    JOIN provinces p ON p.id = d.province_id
    JOIN countries c ON c.id = p.country_id
    WHERE sqlc.arg(level)::text = 'district'
      AND (sqlc.narg(country_iso2)::text IS NULL OR c.iso2 = sqlc.narg(country_iso2)::text)
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR EXISTS (
          SELECT 1 FROM visible_territories t
          WHERE t.country_id = p.country_id
            AND (t.province_id IS NULL OR t.province_id = p.id)
            AND (t.district_id IS NULL OR t.district_id = d.id)
        )
        OR EXISTS (SELECT 1 FROM visible_dealers vd WHERE vd.district_id = d.id)
      )
),
area_stats AS (
    SELECT a.level,
           a.area_id,
           COUNT(d.id)::bigint AS dealer_count,
           COUNT(d.id) FILTER (WHERE d.latitude IS NULL OR d.longitude IS NULL)::bigint AS missing_coordinates,
           AVG(d.latitude)::numeric AS avg_latitude,
           AVG(d.longitude)::numeric AS avg_longitude,
           AVG(pm.value)::numeric AS metric_avg
    FROM areas a
    LEFT JOIN visible_dealers d ON d.country_id = a.country_id
      AND (a.province_id IS NULL OR d.province_id = a.province_id)
      AND (a.district_id IS NULL OR d.district_id = a.district_id)
    LEFT JOIN performance_metrics_monthly pm ON pm.organization_id = d.id
      AND pm.brand_id = sqlc.arg(brand_id)
      AND pm.period = sqlc.arg(period)::text
      AND pm.scope = 'org'
      AND pm.metric = sqlc.arg(metric)::text
    GROUP BY a.level, a.area_id
)
SELECT a.level,
       a.area_id,
       a.code,
       a.name,
       c.iso2::text AS country_iso2,
       c.name_tr AS country_name,
       COALESCE(p.code, '')::text AS province_code,
       p.name AS province_name,
       COALESCE(owner.organization_id, 0)::bigint AS distributor_id,
       COALESCE(owner.organization_uuid, '00000000-0000-0000-0000-000000000000'::uuid) AS distributor_uuid,
       COALESCE(owner.organization_name, '')::text AS distributor_name,
       s.dealer_count,
       s.missing_coordinates,
       s.metric_avg,
       COALESCE(a.area_latitude, s.avg_latitude)::numeric AS latitude,
       COALESCE(a.area_longitude, s.avg_longitude)::numeric AS longitude,
       CASE
         WHEN s.dealer_count = 0 AND owner.organization_id IS NOT NULL THEN 'territory_no_dealers'
         WHEN a.level = 'province' AND owner.organization_id IS NULL THEN 'unassigned_territory'
         ELSE ''
       END::text AS empty_reason
FROM areas a
JOIN area_stats s ON s.level = a.level AND s.area_id = a.area_id
JOIN countries c ON c.id = a.country_id
LEFT JOIN provinces p ON p.id = a.province_id
LEFT JOIN LATERAL (
    SELECT t.organization_id, o.uuid AS organization_uuid, o.name AS organization_name
    FROM visible_territories t
    JOIN organizations o ON o.id = t.organization_id
    WHERE t.country_id = a.country_id
      AND (t.province_id IS NULL OR t.province_id = a.province_id)
      AND (t.district_id IS NULL OR t.district_id = a.district_id)
    ORDER BY (t.district_id IS NOT NULL) DESC, (t.province_id IS NOT NULL) DESC, t.id
    LIMIT 1
) owner ON true
ORDER BY c.iso2, p.code NULLS FIRST, a.code NULLS FIRST, a.name;

-- name: ListPerformanceMapDealers :many
WITH visible_territories AS (
    SELECT t.*
    FROM territories t
    WHERE t.brand_id = sqlc.arg(brand_id)
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR (sqlc.arg(actor_type)::text = 'distributor' AND t.organization_id = sqlc.arg(actor_org_id)::bigint)
      )
),
visible_dealers AS (
    SELECT o.*
    FROM organizations o
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.deleted_at IS NULL
      AND o.type = 'dealer'
      AND (sqlc.narg(country_iso2)::text IS NULL OR EXISTS (
        SELECT 1 FROM countries c WHERE c.id = o.country_id AND c.iso2 = sqlc.narg(country_iso2)::text
      ))
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR (sqlc.arg(actor_type)::text = 'dealer' AND o.id = sqlc.arg(actor_org_id)::bigint)
        OR (
          sqlc.arg(actor_type)::text = 'distributor'
          AND EXISTS (
            SELECT 1
            FROM visible_territories t
            WHERE t.country_id = o.country_id
              AND (t.province_id IS NULL OR t.province_id = o.province_id)
              AND (t.district_id IS NULL OR t.district_id = o.district_id)
          )
        )
      )
)
SELECT d.id AS organization_id,
       d.uuid AS organization_uuid,
       d.slug,
       d.name,
       c.iso2::text AS country_iso2,
       COALESCE(p.code, '')::text AS province_code,
       p.name AS province_name,
       ds.name AS district_name,
       d.latitude,
       d.longitude,
       pm.value::numeric AS metric_value,
       missing.total::bigint AS missing_coordinates
FROM visible_dealers d
JOIN countries c ON c.id = d.country_id
LEFT JOIN provinces p ON p.id = d.province_id
LEFT JOIN districts ds ON ds.id = d.district_id
LEFT JOIN performance_metrics_monthly pm ON pm.organization_id = d.id
  AND pm.brand_id = sqlc.arg(brand_id)
  AND pm.period = sqlc.arg(period)::text
  AND pm.scope = 'org'
  AND pm.metric = sqlc.arg(metric)::text
CROSS JOIN (
    SELECT COUNT(*) FILTER (WHERE latitude IS NULL OR longitude IS NULL) AS total
    FROM visible_dealers
) missing
WHERE d.latitude IS NOT NULL
  AND d.longitude IS NOT NULL
ORDER BY c.iso2, p.code NULLS LAST, d.name, d.id;

-- name: CountPerformanceMapDealerMissingCoordinates :one
WITH visible_territories AS (
    SELECT t.*
    FROM territories t
    WHERE t.brand_id = sqlc.arg(brand_id)
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR (sqlc.arg(actor_type)::text = 'distributor' AND t.organization_id = sqlc.arg(actor_org_id)::bigint)
      )
),
visible_dealers AS (
    SELECT o.*
    FROM organizations o
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.deleted_at IS NULL
      AND o.type = 'dealer'
      AND (sqlc.narg(country_iso2)::text IS NULL OR EXISTS (
        SELECT 1 FROM countries c WHERE c.id = o.country_id AND c.iso2 = sqlc.narg(country_iso2)::text
      ))
      AND (
        sqlc.arg(actor_type)::text = 'center'
        OR (sqlc.arg(actor_type)::text = 'dealer' AND o.id = sqlc.arg(actor_org_id)::bigint)
        OR (
          sqlc.arg(actor_type)::text = 'distributor'
          AND EXISTS (
            SELECT 1
            FROM visible_territories t
            WHERE t.country_id = o.country_id
              AND (t.province_id IS NULL OR t.province_id = o.province_id)
              AND (t.district_id IS NULL OR t.district_id = o.district_id)
          )
        )
      )
)
SELECT COUNT(*) FILTER (WHERE latitude IS NULL OR longitude IS NULL)::bigint
FROM visible_dealers;

-- name: UpsertStaffTarget :one
INSERT INTO staff_targets (
    organization_id, brand_id, user_id, period, metric, value, currency, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(period),
    sqlc.arg(metric), sqlc.arg(value), sqlc.narg(currency), sqlc.narg(created_by_user_id)
)
ON CONFLICT (organization_id, user_id, period, metric) DO UPDATE
SET value = EXCLUDED.value,
    currency = EXCLUDED.currency
RETURNING *;

-- name: GetStaffTarget :one
SELECT * FROM staff_targets
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: DeleteStaffTarget :execrows
DELETE FROM staff_targets
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: ListStaffTargets :many
SELECT st.*,
       u.name AS user_name,
       u.surname AS user_surname
FROM staff_targets st
JOIN users u ON u.id = st.user_id
WHERE st.organization_id = sqlc.arg(organization_id)
  AND st.period >= sqlc.arg(period_from)::text
  AND st.period <= sqlc.arg(period_to)::text
  AND (sqlc.narg(user_id)::bigint IS NULL OR st.user_id = sqlc.narg(user_id)::bigint)
ORDER BY st.period DESC, u.name, u.surname, st.metric, st.id;

-- Bonus rules, settings and accruals ------------------------------------------------

-- name: CreateBonusRule :one
INSERT INTO bonus_rules (
    organization_id, brand_id, name, metric, threshold_pct, kind, amount, percent, currency,
    active, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(metric),
    sqlc.arg(threshold_pct), sqlc.arg(kind), sqlc.narg(amount), sqlc.narg(percent),
    sqlc.narg(currency), sqlc.arg(active), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetBonusRule :one
SELECT * FROM bonus_rules
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: UpdateBonusRule :one
UPDATE bonus_rules
SET name = sqlc.arg(name),
    metric = sqlc.arg(metric),
    threshold_pct = sqlc.arg(threshold_pct),
    kind = sqlc.arg(kind),
    amount = sqlc.narg(amount),
    percent = sqlc.narg(percent),
    currency = sqlc.narg(currency),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: DeleteBonusRule :execrows
-- A rule with accruals is kept (FK RESTRICT); the usecase deactivates it.
DELETE FROM bonus_rules br
WHERE br.id = sqlc.arg(id) AND br.organization_id = sqlc.arg(organization_id)
  AND NOT EXISTS (SELECT 1 FROM bonus_accruals a WHERE a.rule_id = br.id);

-- name: ListBonusRules :many
SELECT * FROM bonus_rules
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
ORDER BY active DESC, threshold_pct, id;

-- name: GetBonusSettings :one
SELECT * FROM bonus_settings
WHERE organization_id = sqlc.arg(organization_id);

-- name: UpsertBonusSettings :one
INSERT INTO bonus_settings (organization_id, brand_id, payout_day)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(payout_day))
ON CONFLICT (organization_id) DO UPDATE
SET payout_day = EXCLUDED.payout_day
RETURNING *;

-- name: UpsertBonusAccrual :one
-- Month-end calculation. A recalculation refreshes a still 'calculated'
-- accrual; approved/posted ones are left untouched (no row returned).
INSERT INTO bonus_accruals (
    organization_id, brand_id, user_id, period, rule_id, achievement_pct, amount, currency
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(period),
    sqlc.arg(rule_id), sqlc.arg(achievement_pct), sqlc.arg(amount), sqlc.arg(currency)
)
ON CONFLICT (user_id, rule_id, period) WHERE status <> 'cancelled' DO UPDATE
SET achievement_pct = EXCLUDED.achievement_pct,
    amount = EXCLUDED.amount,
    currency = EXCLUDED.currency
WHERE bonus_accruals.status = 'calculated'
RETURNING *;

-- name: LockBonusAccrual :one
SELECT * FROM bonus_accruals
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: ApproveBonusAccrual :one
UPDATE bonus_accruals
SET status = 'approved',
    approved_by_user_id = sqlc.narg(approved_by_user_id),
    approved_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'calculated'
RETURNING *;

-- name: MarkBonusAccrualPosted :one
UPDATE bonus_accruals
SET status = 'posted',
    staff_payment_id = sqlc.arg(staff_payment_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'approved'
RETURNING *;

-- name: CancelBonusAccrual :one
UPDATE bonus_accruals
SET status = 'cancelled',
    cancelled_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND status IN ('calculated', 'approved')
RETURNING *;

-- name: ListBonusAccruals :many
SELECT a.*,
       u.name AS user_name,
       u.surname AS user_surname,
       r.name AS rule_name,
       COUNT(*) OVER()::bigint AS total_count
FROM bonus_accruals a
JOIN users u ON u.id = a.user_id
JOIN bonus_rules r ON r.id = a.rule_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(period)::text IS NULL OR a.period = sqlc.narg(period)::text)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR a.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(user_id)::bigint IS NULL OR a.user_id = sqlc.narg(user_id)::bigint)
ORDER BY a.period DESC, u.name, u.surname, a.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Weak dealer rules -------------------------------------------------------------

-- name: CreateWeakDealerRule :one
INSERT INTO weak_dealer_rules (
    organization_id, brand_id, name, metric, operator, threshold, create_task, notify,
    assignee_user_id, active, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(metric),
    sqlc.arg(operator), sqlc.arg(threshold), sqlc.arg(create_task), sqlc.arg(notify),
    sqlc.narg(assignee_user_id), sqlc.arg(active), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetWeakDealerRule :one
SELECT * FROM weak_dealer_rules
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdateWeakDealerRule :one
UPDATE weak_dealer_rules
SET name = sqlc.arg(name),
    metric = sqlc.arg(metric),
    operator = sqlc.arg(operator),
    threshold = sqlc.arg(threshold),
    create_task = sqlc.arg(create_task),
    notify = sqlc.arg(notify),
    assignee_user_id = sqlc.narg(assignee_user_id),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeleteWeakDealerRule :execrows
-- A rule that opened tasks is kept (FK RESTRICT); the usecase deactivates it.
DELETE FROM weak_dealer_rules wr
WHERE wr.id = sqlc.arg(id) AND wr.brand_id = sqlc.arg(brand_id)
  AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.auto_rule_id = wr.id);

-- name: ListWeakDealerRules :many
SELECT r.*,
       o.name AS owner_name,
       o.type AS owner_type
FROM weak_dealer_rules r
JOIN organizations o ON o.id = r.organization_id
WHERE r.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(owner_org_ids)::bigint[] IS NULL OR r.organization_id = ANY (sqlc.narg(owner_org_ids)::bigint[]))
  AND (sqlc.narg(active)::bool IS NULL OR r.active = sqlc.narg(active)::bool)
ORDER BY o.type, r.organization_id, r.name, r.id;

-- Automatic tasks ---------------------------------------------------------------

-- name: InsertAutoPerformanceTask :one
-- One task per subject x rule x month (uq_tasks_auto); a second insert for
-- the same month fails with unique_violation.
INSERT INTO tasks (
    organization_id, brand_id, subject_org_id, title, description,
    assignee_user_id, priority, due_at, source, auto_rule_id, auto_period
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(subject_org_id), sqlc.arg(title),
    sqlc.arg(description), sqlc.narg(assignee_user_id), sqlc.arg(priority), sqlc.narg(due_at),
    'auto', sqlc.arg(auto_rule_id), sqlc.arg(auto_period)
)
RETURNING *;
