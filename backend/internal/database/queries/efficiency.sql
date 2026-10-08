-- TEC-487: efficiency and waste analytics. These queries are intentionally
-- read/projection focused; authorization scope is resolved by the usecase.

-- name: CreatePartConsumptionExpectation :one
INSERT INTO part_consumption_expectations (
    organization_id, brand_id, product_id, category_id, body_type,
    part_key, expected_meters, source, sample_size
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id),
    sqlc.narg(product_id), sqlc.narg(category_id), sqlc.narg(body_type),
    sqlc.arg(part_key), sqlc.arg(expected_meters), sqlc.arg(source), sqlc.arg(sample_size)
)
RETURNING *;

-- name: UpdatePartConsumptionExpectation :one
UPDATE part_consumption_expectations
SET body_type = sqlc.narg(body_type),
    expected_meters = sqlc.arg(expected_meters),
    source = sqlc.arg(source),
    sample_size = sqlc.arg(sample_size)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: GetBestPartExpectation :one
SELECT e.*
FROM part_consumption_expectations e
JOIN products p ON p.id = sqlc.arg(product_id) AND p.brand_id = e.brand_id
WHERE e.brand_id = sqlc.arg(brand_id)
  AND e.part_key = sqlc.arg(part_key)
  AND (e.body_type IS NULL OR e.body_type = sqlc.narg(body_type)::text)
  AND (e.product_id = p.id OR e.category_id = p.category_id)
ORDER BY
  CASE WHEN e.product_id = p.id THEN 0 ELSE 1 END,
  CASE WHEN e.body_type = sqlc.narg(body_type)::text THEN 0 ELSE 1 END,
  e.updated_at DESC,
  e.id DESC
LIMIT 1;

-- name: ListPartConsumptionExpectations :many
SELECT e.*,
       p.name AS product_name,
       p.uuid AS product_uuid,
       c.name AS category_name,
       c.uuid AS category_uuid,
       COUNT(*) OVER()::bigint AS total_count
FROM part_consumption_expectations e
LEFT JOIN products p ON p.id = e.product_id
LEFT JOIN product_categories c ON c.id = COALESCE(e.category_id, p.category_id)
WHERE e.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(q)::text IS NULL
       OR e.part_key ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR c.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(product_ids)::bigint[]), 0) = 0 OR e.product_id = ANY(sqlc.narg(product_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0 OR COALESCE(e.category_id, p.category_id) = ANY(sqlc.narg(category_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(part_keys)::text[]), 0) = 0 OR e.part_key = ANY(sqlc.narg(part_keys)::text[]))
  AND (sqlc.narg(source)::text IS NULL OR e.source = sqlc.narg(source)::text)
  AND (sqlc.narg(body_type)::text IS NULL OR e.body_type = sqlc.narg(body_type)::text)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'part_key' THEN e.part_key
      WHEN 'product' THEN COALESCE(p.name, '')
      WHEN 'category' THEN COALESCE(c.name, '')
      WHEN 'source' THEN e.source
      WHEN 'body_type' THEN COALESCE(e.body_type, '')
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'part_key' THEN e.part_key
      WHEN 'product' THEN COALESCE(p.name, '')
      WHEN 'category' THEN COALESCE(c.name, '')
      WHEN 'source' THEN e.source
      WHEN 'body_type' THEN COALESCE(e.body_type, '')
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'expected_meters' THEN e.expected_meters
      WHEN 'sample_size' THEN e.sample_size::numeric
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'expected_meters' THEN e.expected_meters
      WHEN 'sample_size' THEN e.sample_size::numeric
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'updated_at' THEN e.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'updated_at' THEN e.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN e.id END DESC,
  e.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: DeletePartConsumptionExpectation :execrows
DELETE FROM part_consumption_expectations
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetPartConsumptionExpectationByUUID :one
SELECT * FROM part_consumption_expectations
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: ListEfficiencyServiceItems :many
SELECT si.id
FROM service_items si
JOIN services s ON s.id = si.service_id
WHERE s.status = 'completed'
  AND si.kind = 'partial'
  AND jsonb_array_length(si.applied_parts) > 0
  AND (sqlc.narg(after_id)::bigint IS NULL OR si.id > sqlc.narg(after_id)::bigint)
ORDER BY si.id
LIMIT sqlc.arg(row_limit);

-- name: GetEfficiencyServiceItemUnit :one
SELECT unit_id FROM service_items WHERE id = sqlc.arg(service_item_id);

-- name: RefreshEfficiencyFactsForServiceItem :execrows
WITH deleted AS (
    DELETE FROM efficiency_facts WHERE efficiency_facts.service_item_id = sqlc.arg(target_service_item_id)
),
src AS (
    SELECT si.id AS service_item_id,
           si.service_id,
           si.organization_id,
           si.brand_id,
           si.unit_id,
           si.product_id,
           si.meters,
           si.applied_parts,
           s.organization_id AS dealer_org_id,
           s.performed_by_user_id AS staff_user_id,
           cm.body_type,
           COALESCE(s.completed_at, s.created_at)::date AS service_date,
           p.category_id
    FROM service_items si
    JOIN services s ON s.id = si.service_id
    JOIN products p ON p.id = si.product_id
    JOIN car_models cm ON cm.id = s.car_model_id
    WHERE si.id = sqlc.arg(target_service_item_id)
      AND si.kind = 'partial'
      AND s.status = 'completed'
      AND jsonb_array_length(si.applied_parts) > 0
),
parts AS (
    SELECT src.*,
           part.part_key,
           best.expected_meters
    FROM src
    CROSS JOIN LATERAL jsonb_array_elements_text(src.applied_parts) AS part(part_key)
    LEFT JOIN LATERAL (
        SELECT e.expected_meters
        FROM part_consumption_expectations e
        WHERE e.brand_id = src.brand_id
          AND e.part_key = part.part_key
          AND (e.body_type IS NULL OR e.body_type = src.body_type)
          AND (e.product_id = src.product_id OR e.category_id = src.category_id)
        ORDER BY
          CASE WHEN e.product_id = src.product_id THEN 0 ELSE 1 END,
          CASE WHEN e.body_type = src.body_type THEN 0 ELSE 1 END,
          e.updated_at DESC,
          e.id DESC
        LIMIT 1
    ) best ON true
),
weighted AS (
    SELECT parts.*,
           SUM(COALESCE(parts.expected_meters, 1)) OVER (PARTITION BY parts.service_item_id) AS total_weight
    FROM parts
)
INSERT INTO efficiency_facts (
    organization_id, brand_id, service_id, service_item_id, unit_id,
    product_id, dealer_org_id, staff_user_id, body_type, part_key,
    actual_meters, expected_meters, service_date
)
SELECT organization_id, brand_id, service_id, service_item_id, unit_id,
       product_id, dealer_org_id, staff_user_id, body_type, part_key,
       ROUND((meters * COALESCE(expected_meters, 1) / NULLIF(total_weight, 0))::numeric, 2),
       expected_meters,
       service_date
FROM weighted;

-- name: RebuildRollEfficiency :execrows
WITH affected AS (
    SELECT DISTINCT unit_id FROM efficiency_facts
    WHERE sqlc.narg(unit_id)::bigint IS NULL OR unit_id = sqlc.narg(unit_id)::bigint
    UNION
    SELECT sqlc.narg(unit_id)::bigint
    WHERE sqlc.narg(unit_id)::bigint IS NOT NULL
),
deleted AS (
    DELETE FROM roll_efficiency r
    USING affected a
    WHERE r.unit_id = a.unit_id
),
agg AS (
    SELECT u.organization_id,
           u.brand_id,
           u.id AS unit_id,
           u.product_id,
           u.initial_meters,
           COALESCE(SUM(f.actual_meters), 0)::numeric(10,2) AS consumed_meters,
           COALESCE(SUM(f.expected_meters), 0)::numeric(10,2) AS expected_meters,
           COALESCE(u.remaining_meters, GREATEST(u.initial_meters - COALESCE(SUM(f.actual_meters), 0), 0))::numeric(10,2) AS remaining_meters,
           COUNT(DISTINCT f.service_id)::integer AS service_count,
           MAX(f.service_date)::timestamptz AS last_used_at
    FROM affected a
    JOIN units u ON u.id = a.unit_id
    LEFT JOIN efficiency_facts f ON f.unit_id = u.id
    WHERE u.initial_meters IS NOT NULL
    GROUP BY u.organization_id, u.brand_id, u.id, u.product_id, u.initial_meters, u.remaining_meters
)
INSERT INTO roll_efficiency (
    organization_id, brand_id, unit_id, product_id, initial_meters,
    consumed_meters, expected_meters, remaining_meters, service_count, last_used_at
)
SELECT organization_id, brand_id, unit_id, product_id, initial_meters,
       consumed_meters, expected_meters, remaining_meters, service_count, last_used_at
FROM agg;

-- name: ListRollEfficiency :many
SELECT r.*,
       u.barcode,
       p.name AS product_name,
       CASE WHEN r.expected_meters > 0 THEN (r.consumed_meters / r.expected_meters) - 1 ELSE NULL END::numeric(12,6) AS waste_ratio,
       COUNT(*) OVER()::bigint AS total_count
FROM roll_efficiency r
JOIN units u ON u.id = r.unit_id
JOIN products p ON p.id = r.product_id
WHERE r.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR r.organization_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(unit_uuid)::uuid IS NULL OR u.uuid = sqlc.narg(unit_uuid)::uuid)
  AND (sqlc.narg(q)::text IS NULL OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%' OR p.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(product_ids)::bigint[]), 0) = 0 OR r.product_id = ANY(sqlc.narg(product_ids)::bigint[]))
  AND (sqlc.narg(waste_ratio_min)::numeric IS NULL OR (CASE WHEN r.expected_meters > 0 THEN (r.consumed_meters / r.expected_meters) - 1 ELSE NULL END) >= sqlc.narg(waste_ratio_min)::numeric)
  AND (sqlc.narg(waste_ratio_max)::numeric IS NULL OR (CASE WHEN r.expected_meters > 0 THEN (r.consumed_meters / r.expected_meters) - 1 ELSE NULL END) <= sqlc.narg(waste_ratio_max)::numeric)
  AND (sqlc.narg(last_used_from)::timestamptz IS NULL OR r.last_used_at >= sqlc.narg(last_used_from)::timestamptz)
  AND (sqlc.narg(last_used_to)::timestamptz IS NULL OR r.last_used_at < sqlc.narg(last_used_to)::timestamptz)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'waste_ratio' THEN CASE WHEN r.expected_meters > 0 THEN (r.consumed_meters / r.expected_meters) - 1 ELSE NULL END
      WHEN 'consumed_meters' THEN r.consumed_meters
      WHEN 'remaining_meters' THEN r.remaining_meters
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'waste_ratio' THEN CASE WHEN r.expected_meters > 0 THEN (r.consumed_meters / r.expected_meters) - 1 ELSE NULL END
      WHEN 'consumed_meters' THEN r.consumed_meters
      WHEN 'remaining_meters' THEN r.remaining_meters
    END
  END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_used_at' THEN r.last_used_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_used_at' THEN r.last_used_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN r.id END DESC,
  r.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: EfficiencySummary :many
SELECT CASE sqlc.arg(dimension)::text
         WHEN 'dealer' THEN f.dealer_org_id::text
         WHEN 'staff' THEN f.staff_user_id::text
         WHEN 'product' THEN f.product_id::text
         WHEN 'body_type' THEN COALESCE(f.body_type, '')
         WHEN 'part' THEN f.part_key
       END AS dimension_key,
       CASE sqlc.arg(dimension)::text
         WHEN 'dealer' THEN o.name
         WHEN 'staff' THEN trim(u.name || ' ' || u.surname)
         WHEN 'product' THEN p.name
         WHEN 'body_type' THEN COALESCE(f.body_type, '')
         WHEN 'part' THEN f.part_key
       END AS dimension_label,
       COUNT(DISTINCT f.service_id)::bigint AS service_count,
       SUM(f.actual_meters)::numeric(14,2) AS actual_meters,
       SUM(f.expected_meters)::numeric(14,2) AS expected_meters,
       AVG(f.waste_ratio)::numeric(12,6) AS avg_waste_ratio,
       COUNT(*) OVER()::bigint AS total_count
FROM efficiency_facts f
LEFT JOIN organizations o ON o.id = f.dealer_org_id
LEFT JOIN users u ON u.id = f.staff_user_id
LEFT JOIN products p ON p.id = f.product_id
WHERE f.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR f.dealer_org_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND f.service_date >= sqlc.arg(date_from)::date
  AND f.service_date < sqlc.arg(date_to)::date
GROUP BY dimension_key, dimension_label
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'waste_ratio' THEN AVG(f.waste_ratio)
      WHEN 'meters' THEN SUM(f.actual_meters)
      WHEN 'services' THEN COUNT(DISTINCT f.service_id)::numeric
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'waste_ratio' THEN AVG(f.waste_ratio)
      WHEN 'meters' THEN SUM(f.actual_meters)
      WHEN 'services' THEN COUNT(DISTINCT f.service_id)::numeric
    END
  END DESC NULLS LAST,
  dimension_label ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: EfficiencyMonthlyTrend :many
SELECT date_trunc('month', f.service_date)::date AS month,
       SUM(f.actual_meters)::numeric(14,2) AS actual_meters,
       SUM(f.expected_meters)::numeric(14,2) AS expected_meters,
       AVG(f.waste_ratio)::numeric(12,6) AS avg_waste_ratio,
       COUNT(DISTINCT f.service_id)::bigint AS service_count
FROM efficiency_facts f
WHERE f.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR f.dealer_org_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND f.service_date >= sqlc.arg(date_from)::date
  AND f.service_date < sqlc.arg(date_to)::date
GROUP BY month
ORDER BY month ASC;

-- name: EfficiencyComparison :many
SELECT bucket,
       SUM(actual_meters)::numeric(14,2) AS actual_meters,
       SUM(expected_meters)::numeric(14,2) AS expected_meters,
       AVG(waste_ratio)::numeric(12,6) AS avg_waste_ratio,
       COUNT(DISTINCT service_id)::bigint AS service_count
FROM (
    SELECT 'organization'::text AS bucket, f.*
    FROM efficiency_facts f
    WHERE f.dealer_org_id = sqlc.arg(org_id)
    UNION ALL
    SELECT 'subtree'::text AS bucket, f.*
    FROM efficiency_facts f
    WHERE f.dealer_org_id = ANY(sqlc.arg(subtree_org_ids)::bigint[])
    UNION ALL
    SELECT 'network'::text AS bucket, f.*
    FROM efficiency_facts f
    WHERE f.brand_id = sqlc.arg(brand_id)
) scoped
WHERE service_date >= sqlc.arg(date_from)::date
  AND service_date < sqlc.arg(date_to)::date
GROUP BY bucket
ORDER BY CASE bucket WHEN 'organization' THEN 1 WHEN 'subtree' THEN 2 ELSE 3 END;

-- name: RefreshNetworkPartExpectations :execrows
WITH center_org AS (
    SELECT id, brand_id
    FROM organizations
    WHERE organizations.brand_id = sqlc.arg(brand_id) AND type = 'center' AND deleted_at IS NULL
    ORDER BY id
    LIMIT 1
),
deleted AS (
    DELETE FROM part_consumption_expectations e
    USING center_org c
    WHERE e.brand_id = sqlc.arg(brand_id)
      AND e.organization_id = c.id
      AND e.source = 'network'
    RETURNING 1
),
samples AS (
    SELECT f.brand_id,
           f.product_id,
           p.category_id,
           f.body_type,
           f.part_key,
           percentile_cont(0.5) WITHIN GROUP (ORDER BY f.actual_meters)::numeric(10,2) AS median_meters,
           COUNT(*)::integer AS sample_size
    FROM efficiency_facts f
    JOIN products p ON p.id = f.product_id
    WHERE f.brand_id = sqlc.arg(brand_id)
      AND f.service_date >= sqlc.arg(from_date)::date
      AND f.service_date < sqlc.arg(to_date)::date
    GROUP BY f.brand_id, f.product_id, p.category_id, f.body_type, f.part_key
    HAVING COUNT(*) >= sqlc.arg(min_samples)::integer
),
eligible AS (
    SELECT s.*
    FROM samples s
    WHERE NOT EXISTS (
        SELECT 1
        FROM part_consumption_expectations manual
        WHERE manual.brand_id = s.brand_id
          AND manual.source = 'manual'
          AND manual.part_key = s.part_key
          AND manual.product_id = s.product_id
          AND manual.body_type IS NOT DISTINCT FROM s.body_type
    )
)
INSERT INTO part_consumption_expectations (
    organization_id, brand_id, product_id, category_id, body_type,
    part_key, expected_meters, source, sample_size
)
SELECT c.id, e.brand_id, e.product_id, NULL::bigint, e.body_type,
       e.part_key, e.median_meters, 'network', e.sample_size
FROM eligible e
CROSS JOIN center_org c
CROSS JOIN (SELECT COUNT(*) FROM deleted) deleted_once
WHERE e.median_meters > 0;
