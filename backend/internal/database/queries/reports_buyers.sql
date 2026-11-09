-- TEC-1090 (TEC-1068, TEC-1073): network reports by organization. An
-- order counts once it left draft and was not cancelled (draft, cancelling
-- and cancelled are out); the scope narrows like the other order reports
-- (seller or buyer inside org_ids). The active organization's own
-- purchases are not a buyer row (self_org_id).

-- name: ReportOrdersByBuyer :many
SELECT b.uuid AS organization_uuid, b.name AS organization_name, b.type AS organization_type, b.city,
       COUNT(*)::bigint AS order_count,
       SUM(o.total)::numeric(18,2)::text AS total,
       MAX(o.currency)::text AS currency,
       MAX(o.created_at)::timestamptz AS last_order_at
FROM orders o
JOIN organizations b ON b.id = o.buyer_org_id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR o.buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR o.created_by_user_id = sqlc.narg(created_by_user_id))
  AND o.buyer_org_id <> sqlc.arg(self_org_id)::bigint
  AND o.status NOT IN ('draft', 'cancelling', 'cancelled')
  AND o.created_at >= sqlc.arg(range_from)::timestamptz
  AND o.created_at < sqlc.arg(range_to)::timestamptz
GROUP BY b.id, b.uuid, b.name, b.type, b.city
ORDER BY order_count DESC, SUM(o.total) DESC, lower(b.name), b.id
LIMIT sqlc.arg(row_limit);

-- name: ReportInactiveBuyers :many
-- Active dealers inside the scope with no counted order in the range
-- (all_time: none at all); last_order_at is their latest counted order
-- ever (null: never ordered). Oldest activity first, never-ordered first.
-- parent_id narrows to the direct dealers of a managed reach (a
-- distributor's own customers).
WITH last_orders AS (
    SELECT o.buyer_org_id, MAX(o.created_at) AS last_order_at,
           COUNT(*) FILTER (WHERE o.created_at >= sqlc.arg(range_from)::timestamptz
                              AND o.created_at < sqlc.arg(range_to)::timestamptz) AS in_range
    FROM orders o
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.status NOT IN ('draft', 'cancelling', 'cancelled')
    GROUP BY o.buyer_org_id
)
SELECT d.uuid AS organization_uuid, d.name AS organization_name, d.city,
       lo.last_order_at::timestamptz AS last_order_at,
       COUNT(*) OVER ()::bigint AS total_count
FROM organizations d
LEFT JOIN last_orders lo ON lo.buyer_org_id = d.id
WHERE d.brand_id = sqlc.arg(brand_id)
  AND d.type = 'dealer'
  AND d.status = 'active'
  AND d.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR d.id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(parent_id)::bigint IS NULL OR d.parent_id = sqlc.narg(parent_id)::bigint)
  AND (CASE WHEN sqlc.arg(all_time)::bool THEN lo.buyer_org_id IS NULL
            ELSE COALESCE(lo.in_range, 0) = 0 END)
ORDER BY lo.last_order_at ASC NULLS FIRST, lower(d.name), d.id
LIMIT sqlc.arg(row_limit);

-- name: ReportServicesByOrg :many
-- Services completed in the range per organization (same basis as the
-- other service rankings). The center's own row carries its service point
-- (TEC-1003) so a client can label it apart from the dealers.
SELECT o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type, o.city,
       o.service_point_enabled, o.service_point_name,
       COUNT(*)::bigint AS service_count
FROM services s
JOIN organizations o ON o.id = s.organization_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.completed_at >= sqlc.arg(range_from)::timestamptz
  AND s.completed_at < sqlc.arg(range_to)::timestamptz
GROUP BY o.id, o.uuid, o.name, o.type, o.city, o.service_point_enabled, o.service_point_name
ORDER BY service_count DESC, lower(o.name), o.id
LIMIT sqlc.arg(row_limit);
