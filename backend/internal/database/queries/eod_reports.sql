-- TEC-207: end-of-day warehouse reports (000071). Every query is bound to
-- one organization; the warehouse side is brand-independent (K20).

-- name: SummarizeEODMovements :many
-- The day's ledger movements of an organization grouped by type and
-- product. A movement belongs to the organization when it was recorded on
-- it (holder before the movement) or when it lands on one of its locations
-- or on the organization itself. With warehouse_id, only movements that
-- leave or reach a location of that warehouse, or that a stock entry of
-- that warehouse wrote (serial entries go to the organization first), and
-- warehouse transfer receipts of that target warehouse (TEC-205: the
-- transfer_in lands on the organization before its placement).
SELECT m.type::text AS type,
       p.id AS product_id,
       p.uuid AS product_uuid,
       p.sku AS sku,
       p.name AS product_name,
       COUNT(*)::bigint AS movement_count,
       COUNT(DISTINCT m.unit_id)::bigint AS unit_count,
       COALESCE(SUM(GREATEST(m.quantity_delta, 0)), 0)::bigint AS quantity_in,
       COALESCE(SUM(GREATEST(-m.quantity_delta, 0)), 0)::bigint AS quantity_out,
       COALESCE(SUM(GREATEST(m.meters_delta, 0)), 0)::numeric(14,2)::text AS meters_in,
       COALESCE(SUM(GREATEST(-m.meters_delta, 0)), 0)::numeric(14,2)::text AS meters_out
FROM stock_movements m
JOIN products p ON p.id = m.product_id
LEFT JOIN warehouse_locations fl
       ON m.from_owner_type = 'warehouse_location' AND fl.id = m.from_owner_id
LEFT JOIN warehouse_locations tl
       ON m.to_owner_type = 'warehouse_location' AND tl.id = m.to_owner_id
LEFT JOIN stock_entry_lines sel
       ON m.reference_type = 'stock_entry_line' AND sel.id = m.reference_id
LEFT JOIN stock_entries se
       ON se.id = sel.entry_id AND se.organization_id = sqlc.arg(organization_id)::bigint
LEFT JOIN warehouse_transfer_lines wtl
       ON m.reference_type = 'warehouse_transfer_line' AND wtl.id = m.reference_id
LEFT JOIN warehouse_transfers wt
       ON wt.id = wtl.transfer_id AND wt.organization_id = sqlc.arg(organization_id)::bigint
WHERE m.created_at >= sqlc.arg(period_start)::timestamptz
  AND m.created_at < sqlc.arg(period_end)::timestamptz
  AND (m.organization_id = sqlc.arg(organization_id)::bigint
       OR tl.organization_id = sqlc.arg(organization_id)::bigint
       OR (m.to_owner_type = 'organization' AND m.to_owner_id = sqlc.arg(organization_id)::bigint))
  AND (sqlc.narg(warehouse_id)::bigint IS NULL
       OR (fl.organization_id = sqlc.arg(organization_id)::bigint AND fl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
       OR (tl.organization_id = sqlc.arg(organization_id)::bigint AND tl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
       OR se.warehouse_id = sqlc.narg(warehouse_id)::bigint
       OR (m.type = 'transfer_in' AND wt.to_warehouse_id = sqlc.narg(warehouse_id)::bigint))
GROUP BY m.type, p.id, p.uuid, p.sku, p.name
ORDER BY m.type, p.name, p.id;

-- name: UpsertEODReport :one
-- Manual run: (re)writes the report of the scope and day.
INSERT INTO eod_reports (
    organization_id, brand_id, warehouse_id, report_date, timezone,
    period_start, period_end, kind, summary, generated_by_user_id, generated_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(warehouse_id), sqlc.arg(report_date),
    sqlc.arg(timezone), sqlc.arg(period_start), sqlc.arg(period_end), sqlc.arg(kind),
    sqlc.arg(summary), sqlc.narg(generated_by_user_id), NOW()
)
ON CONFLICT ON CONSTRAINT uq_eod_reports_scope DO UPDATE
SET timezone = EXCLUDED.timezone,
    period_start = EXCLUDED.period_start,
    period_end = EXCLUDED.period_end,
    kind = EXCLUDED.kind,
    summary = EXCLUDED.summary,
    generated_by_user_id = EXCLUDED.generated_by_user_id,
    generated_at = NOW()
RETURNING *;

-- name: InsertEODReportIfMissing :one
-- Cron run: writes the report only when the scope and day has none (no
-- row returned otherwise), so a rerun or a manual report wins.
INSERT INTO eod_reports (
    organization_id, brand_id, warehouse_id, report_date, timezone,
    period_start, period_end, kind, summary, generated_by_user_id, generated_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(warehouse_id), sqlc.arg(report_date),
    sqlc.arg(timezone), sqlc.arg(period_start), sqlc.arg(period_end), sqlc.arg(kind),
    sqlc.arg(summary), sqlc.narg(generated_by_user_id), NOW()
)
ON CONFLICT ON CONSTRAINT uq_eod_reports_scope DO NOTHING
RETURNING *;

-- name: EODReportExists :one
SELECT EXISTS (
    SELECT 1 FROM eod_reports
    WHERE organization_id = sqlc.arg(organization_id)
      AND warehouse_id IS NOT DISTINCT FROM sqlc.narg(warehouse_id)::bigint
      AND report_date = sqlc.arg(report_date)
)::bool AS exists;

-- name: GetEODReportByUUID :one
SELECT * FROM eod_reports
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListEODReports :many
-- scope: '' every report, 'system' only system reports, 'warehouse' only
-- warehouse reports (warehouse_id narrows to one warehouse).
-- TEC-375: sort keys from warehouse usecase EODSort (report_date,
-- generated_at); within one key the system report comes first.
SELECT * FROM eod_reports
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(warehouse_id)::bigint IS NULL OR warehouse_id = sqlc.narg(warehouse_id)::bigint)
  AND (sqlc.arg(scope)::text = ''
       OR (sqlc.arg(scope)::text = 'system' AND warehouse_id IS NULL)
       OR (sqlc.arg(scope)::text = 'warehouse' AND warehouse_id IS NOT NULL))
  AND (sqlc.narg(date_from)::date IS NULL OR report_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR report_date <= sqlc.narg(date_to)::date)
  AND (COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0 OR kind = ANY (sqlc.narg(kinds)::text[]))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'report_date' THEN report_date END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'report_date' THEN report_date END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'generated_at' THEN generated_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'generated_at' THEN generated_at END END DESC,
  warehouse_id NULLS FIRST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountEODReports :one
SELECT COUNT(*)::bigint FROM eod_reports
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(warehouse_id)::bigint IS NULL OR warehouse_id = sqlc.narg(warehouse_id)::bigint)
  AND (sqlc.arg(scope)::text = ''
       OR (sqlc.arg(scope)::text = 'system' AND warehouse_id IS NULL)
       OR (sqlc.arg(scope)::text = 'warehouse' AND warehouse_id IS NOT NULL))
  AND (sqlc.narg(date_from)::date IS NULL OR report_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR report_date <= sqlc.narg(date_to)::date)
  AND (COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0 OR kind = ANY (sqlc.narg(kinds)::text[]));

-- name: ListEODReportOrganizations :many
-- The organizations the cron reports on: active centers and distributors
-- with at least one active warehouse.
SELECT o.id, o.brand_id, o.timezone
FROM organizations o
WHERE o.type IN ('center', 'distributor')
  AND o.status = 'active'
  AND o.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM warehouses w WHERE w.organization_id = o.id AND w.active)
ORDER BY o.id;
