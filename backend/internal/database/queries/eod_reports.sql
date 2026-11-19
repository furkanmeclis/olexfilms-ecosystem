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

-- name: EODCurrentStock :many
-- TEC-1229: current on-hand stock per product of the organization
-- (warehouse_id NULL: organization_product_stocks, the whole organization)
-- or of one warehouse (bin_product_stocks of its locations).
SELECT s.product_id, p.uuid AS product_uuid, p.sku, p.name AS product_name,
       s.quantity::bigint AS quantity, s.meters::numeric(14,2)::text AS meters
FROM (
    SELECT ops.product_id, ops.quantity::bigint AS quantity, ops.meters::numeric(14,2) AS meters
    FROM organization_product_stocks ops
    WHERE sqlc.narg(warehouse_id)::bigint IS NULL
      AND ops.organization_id = sqlc.arg(organization_id)::bigint
    UNION ALL
    SELECT b.product_id, SUM(b.quantity)::bigint, SUM(b.meters)::numeric(14,2)
    FROM bin_product_stocks b
    JOIN warehouse_locations wl ON wl.id = b.location_id
    WHERE sqlc.narg(warehouse_id)::bigint IS NOT NULL
      AND wl.warehouse_id = sqlc.narg(warehouse_id)::bigint
      AND b.organization_id = sqlc.arg(organization_id)::bigint
    GROUP BY b.product_id
) s
JOIN products p ON p.id = s.product_id;

-- name: EODCurrentMeteredUnits :many
-- TEC-1243: current metered serial units on hand per product of the
-- organization or of one warehouse; the closed boxes of a balance are its
-- quantity minus these (EODBalances).
SELECT u.product_id, COUNT(*)::bigint AS rolls
FROM unit_current_state st
JOIN units u ON u.id = st.unit_id
LEFT JOIN warehouse_locations wl ON wl.id = st.owner_location_id
WHERE st.status IN ('available', 'placed')
  AND st.holder_org_id = sqlc.arg(organization_id)::bigint
  AND st.owner_type IN ('warehouse_location', 'organization')
  AND u.initial_meters IS NOT NULL
  AND (sqlc.narg(warehouse_id)::bigint IS NULL OR wl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
GROUP BY u.product_id;

-- name: EODStockDeltas :many
-- TEC-1229: the on-hand change per product caused by the movements since
-- period_start, split at period_end (period: [start, end), after: [end,
-- now)). A movement adds its "after" side when the unit is counted in the
-- scope after it and removes its "before" side when it was counted there
-- before it (reclassification moves the stock from metadata
-- from_product_id to product_id). Rolls carry their remaining meters (the
-- unit's initial meters plus every meters_delta up to the movement, as the
-- rebuild replay does). Fixed barcodes change one owner per movement: a
-- negative quantity_delta debits from_owner, a positive one credits
-- to_owner. Opening and closing = current - after - period / current -
-- after (EODBalances).
WITH mv AS (
    SELECT m.id, m.unit_id, m.created_at, m.product_id, m.quantity_delta, m.meters_delta,
           COALESCE(CASE WHEN m.type = 'reclassification' THEN (m.metadata->>'from_product_id')::bigint END,
                    m.product_id) AS before_product_id,
           u.unit_kind,
           (u.unit_kind = 'serial' AND u.initial_meters IS NOT NULL) AS is_roll,
           CASE WHEN u.unit_kind = 'serial' AND u.initial_meters IS NOT NULL THEN
               u.initial_meters + (SELECT COALESCE(SUM(k.meters_delta), 0) FROM stock_movements k
                                   WHERE k.unit_id = m.unit_id AND k.id <= m.id)
           END AS remaining_after,
           (m.from_status IN ('available', 'placed')) AS from_counted,
           (m.to_status IN ('available', 'placed')) AS to_counted,
           CASE
               WHEN m.from_owner_type = 'warehouse_location' THEN
                   fl.organization_id = sqlc.arg(organization_id)::bigint
                   AND (sqlc.narg(warehouse_id)::bigint IS NULL OR fl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
               WHEN m.from_owner_type = 'organization' THEN
                   sqlc.narg(warehouse_id)::bigint IS NULL AND m.from_owner_id = sqlc.arg(organization_id)::bigint
               ELSE false
           END AS from_in,
           CASE
               WHEN m.to_owner_type = 'warehouse_location' THEN
                   tl.organization_id = sqlc.arg(organization_id)::bigint
                   AND (sqlc.narg(warehouse_id)::bigint IS NULL OR tl.warehouse_id = sqlc.narg(warehouse_id)::bigint)
               WHEN m.to_owner_type = 'organization' THEN
                   sqlc.narg(warehouse_id)::bigint IS NULL AND m.to_owner_id = sqlc.arg(organization_id)::bigint
               ELSE false
           END AS to_in
    FROM stock_movements m
    JOIN units u ON u.id = m.unit_id
    LEFT JOIN warehouse_locations fl ON m.from_owner_type = 'warehouse_location' AND fl.id = m.from_owner_id
    LEFT JOIN warehouse_locations tl ON m.to_owner_type = 'warehouse_location' AND tl.id = m.to_owner_id
    WHERE m.created_at >= sqlc.arg(period_start)::timestamptz
      AND (fl.organization_id = sqlc.arg(organization_id)::bigint
           OR tl.organization_id = sqlc.arg(organization_id)::bigint
           OR (m.from_owner_type = 'organization' AND m.from_owner_id = sqlc.arg(organization_id)::bigint)
           OR (m.to_owner_type = 'organization' AND m.to_owner_id = sqlc.arg(organization_id)::bigint))
), parts AS (
    SELECT mv.product_id AS pid, mv.created_at,
           CASE
               WHEN mv.unit_kind = 'fixed' THEN CASE WHEN mv.quantity_delta > 0 AND mv.to_in THEN mv.quantity_delta ELSE 0 END
               WHEN mv.to_in AND mv.to_counted THEN 1 ELSE 0
           END::bigint AS qty,
           CASE WHEN mv.is_roll AND mv.to_in AND mv.to_counted THEN mv.remaining_after ELSE 0 END::numeric(14,2) AS meters,
           CASE WHEN mv.is_roll AND mv.to_in AND mv.to_counted THEN 1 ELSE 0 END::bigint AS rolls
    FROM mv
    UNION ALL
    SELECT mv.before_product_id AS pid, mv.created_at,
           CASE
               WHEN mv.unit_kind = 'fixed' THEN CASE WHEN mv.quantity_delta < 0 AND mv.from_in THEN mv.quantity_delta ELSE 0 END
               WHEN mv.from_in AND mv.from_counted THEN -1 ELSE 0
           END::bigint AS qty,
           CASE WHEN mv.is_roll AND mv.from_in AND mv.from_counted THEN -(mv.remaining_after - mv.meters_delta) ELSE 0 END::numeric(14,2) AS meters,
           CASE WHEN mv.is_roll AND mv.from_in AND mv.from_counted THEN -1 ELSE 0 END::bigint AS rolls
    FROM mv
)
SELECT parts.pid AS product_id, p.uuid AS product_uuid, p.sku, p.name AS product_name,
       COALESCE(SUM(parts.qty) FILTER (WHERE parts.created_at < sqlc.arg(period_end)::timestamptz), 0)::bigint AS period_quantity,
       COALESCE(SUM(parts.meters) FILTER (WHERE parts.created_at < sqlc.arg(period_end)::timestamptz), 0)::numeric(14,2)::text AS period_meters,
       COALESCE(SUM(parts.qty) FILTER (WHERE parts.created_at >= sqlc.arg(period_end)::timestamptz), 0)::bigint AS after_quantity,
       COALESCE(SUM(parts.meters) FILTER (WHERE parts.created_at >= sqlc.arg(period_end)::timestamptz), 0)::numeric(14,2)::text AS after_meters,
       COALESCE(SUM(parts.rolls) FILTER (WHERE parts.created_at < sqlc.arg(period_end)::timestamptz), 0)::bigint AS period_rolls,
       COALESCE(SUM(parts.rolls) FILTER (WHERE parts.created_at >= sqlc.arg(period_end)::timestamptz), 0)::bigint AS after_rolls
FROM parts
JOIN products p ON p.id = parts.pid
GROUP BY parts.pid, p.uuid, p.sku, p.name;

-- name: EODPendingWork :one
-- TEC-1229: open work of the organization (or one warehouse) at report
-- time: open warehouse transfers (draft / in transit, from or to the
-- warehouse), draft stock entries and confirmed entries with lines still
-- waiting for a location, unfinished stock counts and sale orders being
-- prepared (approved / preparing / ready).
SELECT
    (SELECT COUNT(*) FROM warehouse_transfers t
     WHERE t.organization_id = sqlc.arg(organization_id)::bigint
       AND t.status IN ('draft', 'in_transit')
       AND (sqlc.narg(warehouse_id)::bigint IS NULL
            OR t.from_warehouse_id = sqlc.narg(warehouse_id)::bigint
            OR t.to_warehouse_id = sqlc.narg(warehouse_id)::bigint))::bigint AS open_transfers,
    (SELECT COUNT(*) FROM stock_entries e
     WHERE e.organization_id = sqlc.arg(organization_id)::bigint
       AND (sqlc.narg(warehouse_id)::bigint IS NULL OR e.warehouse_id = sqlc.narg(warehouse_id)::bigint)
       AND (e.status = 'draft'
            OR (e.status = 'confirmed' AND EXISTS (
                SELECT 1 FROM stock_entry_lines l
                JOIN unit_current_state st ON st.unit_id = l.unit_id
                WHERE l.entry_id = e.id
                  AND l.placement_movement_id IS NULL
                  AND l.undo_movement_id IS NULL
                  AND st.owner_type = 'organization'
                  AND st.holder_org_id = e.organization_id
                  AND st.status = 'available'))))::bigint AS unplaced_entries,
    (SELECT COUNT(*) FROM stock_counts c
     WHERE c.organization_id = sqlc.arg(organization_id)::bigint
       AND c.status IN ('draft', 'in_progress', 'pending_review')
       AND (sqlc.narg(warehouse_id)::bigint IS NULL OR c.warehouse_id = sqlc.narg(warehouse_id)::bigint))::bigint AS pending_counts,
    (SELECT COUNT(*) FROM orders o
     WHERE o.seller_org_id = sqlc.arg(organization_id)::bigint
       AND o.status IN ('approved', 'preparing', 'ready')
       AND (sqlc.narg(warehouse_id)::bigint IS NULL OR o.seller_warehouse_id = sqlc.narg(warehouse_id)::bigint))::bigint AS preparing_orders;
