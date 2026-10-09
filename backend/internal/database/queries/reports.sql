-- TEC-495 (F5-05f): /v1/reports aggregates (mobile reports and panel
-- widgets). Scope arguments follow scopefilter like service_stats.sql:
-- brand_id is always the domain brand of the active organization (K20),
-- org_ids NULL = whole brand (brand / all scope) else the organizations in
-- reach, created_by_user_id narrows own / assigned grants. Ranges are
-- half open [range_from, range_to); buckets are date_trunc(granularity)
-- of the timestamp in the caller's timezone, gaps are filled by the
-- usecase.

-- name: ReportServiceTrend :many
SELECT 'created'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, s.created_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM services s
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.created_at >= sqlc.arg(range_from)::timestamptz
  AND s.created_at < sqlc.arg(range_to)::timestamptz
GROUP BY 2
UNION ALL
SELECT 'completed'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, s.completed_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM services s
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.completed_at >= sqlc.arg(range_from)::timestamptz
  AND s.completed_at < sqlc.arg(range_to)::timestamptz
GROUP BY 2;

-- name: ReportServiceStatuses :many
SELECT s.status, COUNT(*)::bigint AS value
FROM services s
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.created_at >= sqlc.arg(range_from)::timestamptz
  AND s.created_at < sqlc.arg(range_to)::timestamptz
GROUP BY s.status;

-- name: ReportServiceOverview :one
SELECT
    COUNT(*) FILTER (WHERE s.created_at >= sqlc.arg(range_from)::timestamptz
                       AND s.created_at < sqlc.arg(range_to)::timestamptz)::bigint AS created_count,
    COUNT(*) FILTER (WHERE s.completed_at >= sqlc.arg(range_from)::timestamptz
                       AND s.completed_at < sqlc.arg(range_to)::timestamptz)::bigint AS completed_count,
    COUNT(*) FILTER (WHERE s.status IN ('pending', 'processing', 'ready'))::bigint AS open_count
FROM services s
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id));

-- name: ReportServiceTopModels :many
SELECT cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cm.uuid AS car_model_uuid, cm.name AS car_model_name,
       COUNT(*)::bigint AS service_count
FROM services s
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.completed_at >= sqlc.arg(range_from)::timestamptz
  AND s.completed_at < sqlc.arg(range_to)::timestamptz
GROUP BY cb.id, cb.uuid, cb.name, cm.id, cm.uuid, cm.name
ORDER BY service_count DESC, lower(cb.name), lower(cm.name), cm.id
LIMIT sqlc.arg(row_limit);

-- name: ReportRecentServices :many
SELECT s.uuid, s.service_no, s.plate, s.status, s.created_at, s.completed_at,
       o.uuid AS organization_uuid, o.name AS organization_name,
       cb.name AS car_brand_name, cm.name AS car_model_name
FROM services s
JOIN organizations o ON o.id = s.organization_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(row_limit);

-- Orders: the caller reaches an order as its owner (organization_id) or
-- its buyer, like ListOrders side=all.

-- name: ReportOrderTrend :many
SELECT 'created'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, o.created_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR o.buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR o.created_by_user_id = sqlc.narg(created_by_user_id))
  AND o.created_at >= sqlc.arg(range_from)::timestamptz
  AND o.created_at < sqlc.arg(range_to)::timestamptz
GROUP BY 2
UNION ALL
SELECT 'received'::text AS series,
       date_trunc(sqlc.arg(granularity)::text, o.received_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR o.buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR o.created_by_user_id = sqlc.narg(created_by_user_id))
  AND o.received_at >= sqlc.arg(range_from)::timestamptz
  AND o.received_at < sqlc.arg(range_to)::timestamptz
GROUP BY 2;

-- name: ReportOrderStatuses :many
SELECT o.status, COUNT(*)::bigint AS value
FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR o.buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR o.created_by_user_id = sqlc.narg(created_by_user_id))
  AND o.created_at >= sqlc.arg(range_from)::timestamptz
  AND o.created_at < sqlc.arg(range_to)::timestamptz
GROUP BY o.status;

-- name: ReportOrderOverview :one
SELECT
    COUNT(*) FILTER (WHERE o.created_at >= sqlc.arg(range_from)::timestamptz
                       AND o.created_at < sqlc.arg(range_to)::timestamptz)::bigint AS created_count,
    COUNT(*) FILTER (WHERE o.status NOT IN ('draft', 'delivered', 'received', 'cancelled'))::bigint AS open_count
FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR o.buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR o.created_by_user_id = sqlc.narg(created_by_user_id));

-- Customers: a customer belongs to the organizations it is linked to
-- (customer_organizations); type comes from customer_profiles.

-- name: ReportCustomerTrend :many
SELECT COALESCE(cp.type, 'individual')::text AS series,
       date_trunc(sqlc.arg(granularity)::text, co.created_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(DISTINCT co.user_id)::bigint AS value
FROM customer_organizations co
LEFT JOIN customer_profiles cp ON cp.user_id = co.user_id
WHERE co.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND co.created_at >= sqlc.arg(range_from)::timestamptz
  AND co.created_at < sqlc.arg(range_to)::timestamptz
GROUP BY 1, 2;

-- name: ReportCustomerOverview :one
SELECT
    COUNT(DISTINCT co.user_id)::bigint AS total_count,
    COUNT(DISTINCT co.user_id) FILTER (WHERE co.created_at >= sqlc.arg(range_from)::timestamptz
                                         AND co.created_at < sqlc.arg(range_to)::timestamptz)::bigint AS new_count
FROM customer_organizations co
WHERE co.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]));

-- Stock: a snapshot of the holding organizations in reach (serial units by
-- status from unit_current_state, quantities from organization_product_stocks).

-- name: ReportStockUnitStatuses :many
SELECT u.status, COUNT(*)::bigint AS value
FROM unit_current_state ucs
JOIN units u ON u.id = ucs.unit_id
WHERE ucs.brand_id = sqlc.arg(brand_id)
  AND ucs.holder_org_id IS NOT NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR ucs.holder_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
GROUP BY u.status;

-- name: ReportStockTotals :one
SELECT
    COUNT(DISTINCT s.product_id) FILTER (WHERE s.quantity > 0 OR s.meters > 0)::bigint AS product_count,
    COALESCE(SUM(s.quantity), 0)::bigint AS quantity,
    COALESCE(SUM(s.meters), 0)::numeric(14,2)::float8 AS meters
FROM organization_product_stocks s
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]));

-- Warranties: own / assigned grants reach the warranties of their own
-- services.

-- name: ReportWarrantyCounts :one
SELECT
    COUNT(*) FILTER (WHERE w.status = 'active')::bigint AS active_count,
    COUNT(*) FILTER (WHERE w.status = 'expired')::bigint AS expired_count,
    COUNT(*) FILTER (WHERE w.status = 'void')::bigint AS void_count,
    COUNT(*) FILTER (WHERE w.status = 'active'
                       AND w.end_at >= sqlc.arg(now_at)::timestamptz
                       AND w.end_at < sqlc.arg(now_at)::timestamptz + interval '30 days')::bigint AS expiring_count,
    COUNT(*) FILTER (WHERE w.start_at >= sqlc.arg(range_from)::timestamptz
                       AND w.start_at < sqlc.arg(range_to)::timestamptz)::bigint AS started_count
FROM warranties w
WHERE w.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM services s WHERE s.id = w.service_id AND s.created_by_user_id = sqlc.narg(created_by_user_id)));

-- name: ReportWarrantyTrend :many
SELECT date_trunc(sqlc.arg(granularity)::text, w.start_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM warranties w
WHERE w.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM services s WHERE s.id = w.service_id AND s.created_by_user_id = sqlc.narg(created_by_user_id)))
  AND w.start_at >= sqlc.arg(range_from)::timestamptz
  AND w.start_at < sqlc.arg(range_to)::timestamptz
GROUP BY 1;

-- name: ReportTopDealersByWarranty :many
SELECT o.uuid AS organization_uuid, o.name AS organization_name, o.city,
       COUNT(*)::bigint AS warranty_count,
       COUNT(*) FILTER (WHERE w.status = 'active')::bigint AS active_count
FROM warranties w
JOIN organizations o ON o.id = w.organization_id
WHERE w.brand_id = sqlc.arg(brand_id)
  AND o.type = 'dealer'
  AND o.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND w.start_at >= sqlc.arg(range_from)::timestamptz
  AND w.start_at < sqlc.arg(range_to)::timestamptz
GROUP BY o.id, o.uuid, o.name, o.city
ORDER BY warranty_count DESC, lower(o.name), o.id
LIMIT sqlc.arg(row_limit);

-- Measurements (NexPTG results) by measured_at.

-- name: ReportMeasurementTrend :many
SELECT date_trunc(sqlc.arg(granularity)::text, m.measured_at AT TIME ZONE sqlc.arg(tz)::text)::date AS bucket,
       COUNT(*)::bigint AS value
FROM measurement_results m
WHERE m.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR m.created_by = sqlc.narg(created_by_user_id))
  AND m.measured_at >= sqlc.arg(range_from)::timestamptz
  AND m.measured_at < sqlc.arg(range_to)::timestamptz
GROUP BY 1;

-- name: ReportMeasurementCounts :one
SELECT
    COUNT(*)::bigint AS total_count,
    COUNT(*) FILTER (WHERE m.status = 'accepted')::bigint AS accepted_count,
    COUNT(*) FILTER (WHERE m.status = 'vin_pending')::bigint AS vin_pending_count,
    COUNT(*) FILTER (WHERE m.service_id IS NOT NULL)::bigint AS linked_count,
    COUNT(DISTINCT m.vin) FILTER (WHERE m.vin IS NOT NULL)::bigint AS vehicle_count
FROM measurement_results m
WHERE m.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR m.created_by = sqlc.narg(created_by_user_id))
  AND m.measured_at >= sqlc.arg(range_from)::timestamptz
  AND m.measured_at < sqlc.arg(range_to)::timestamptz;

-- name: ReportNetworkCounts :one
SELECT
    COUNT(*)::bigint AS dealer_count,
    COUNT(*) FILTER (WHERE o.status = 'active')::bigint AS active_dealer_count
FROM organizations o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.type = 'dealer'
  AND o.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(org_ids)::bigint[]));

-- name: GetReportLayout :one
SELECT * FROM report_layouts
WHERE user_id = sqlc.arg(user_id) AND organization_id = sqlc.arg(organization_id);

-- name: UpsertReportLayout :one
INSERT INTO report_layouts (user_id, organization_id, brand_id, version, widgets)
VALUES (sqlc.arg(user_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(version), sqlc.arg(widgets))
ON CONFLICT (user_id, organization_id) DO UPDATE
SET version = EXCLUDED.version, widgets = EXCLUDED.widgets, brand_id = EXCLUDED.brand_id, updated_at = NOW()
RETURNING *;
