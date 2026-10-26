-- TEC-1008: bulk barcode operations.

-- name: ListStockBulkCandidates :many
-- Units issued by the organization that match the selection, with what the
-- eligibility rules need. row_limit is the job limit + 1.
SELECT u.id, u.uuid, u.barcode, u.unit_kind, u.status AS unit_status, u.product_id, u.brand_id,
       u.initial_meters, u.remaining_meters, u.lot, u.note,
       p.default_roll_meters, p.uses_fixed_barcode,
       COALESCE(st.owner_type, '')::text AS owner_type, COALESCE(st.owner_id, 0)::bigint AS owner_id,
       COALESCE(st.holder_org_id, 0)::bigint AS holder_org_id, COALESCE(st.status, '')::text AS state_status,
       COALESCE(st.last_movement_id, 0)::bigint AS last_movement_id,
       EXISTS (SELECT 1 FROM service_items si JOIN services s ON s.id = si.service_id
               WHERE si.unit_id = u.id AND s.status <> 'cancelled') AS in_service,
       EXISTS (SELECT 1 FROM stock_reservations r WHERE r.unit_id = u.id AND r.status = 'active') AS reserved
FROM units u
JOIN products p ON p.id = u.product_id
LEFT JOIN unit_current_state st ON st.unit_id = u.id
LEFT JOIN warehouse_locations loc ON st.owner_type = 'warehouse_location' AND loc.id = st.owner_id
WHERE u.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(barcodes)::text[]), 0) = 0 OR u.barcode = ANY (sqlc.narg(barcodes)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(product_ids)::bigint[]), 0) = 0 OR u.product_id = ANY (sqlc.narg(product_ids)::bigint[]))
  AND (sqlc.narg(warehouse_id)::bigint IS NULL OR loc.warehouse_id = sqlc.narg(warehouse_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(location_ids)::bigint[]), 0) = 0 OR loc.id = ANY (sqlc.narg(location_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
       OR COALESCE(st.status, u.status) = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(lot)::text IS NULL OR u.lot = sqlc.narg(lot)::text)
  AND (sqlc.narg(entry_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM stock_entry_lines el WHERE el.unit_id = u.id AND el.entry_id = sqlc.narg(entry_id)::bigint))
ORDER BY u.id
LIMIT sqlc.arg(row_limit);

-- name: CreateStockBulkJob :one
INSERT INTO stock_bulk_jobs (organization_id, action, params, selection, total, eligible, excluded, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(action), sqlc.arg(params), sqlc.arg(selection),
        sqlc.arg(total), sqlc.arg(eligible), sqlc.arg(excluded), sqlc.arg(created_by))
RETURNING *;

-- name: InsertStockBulkJobItems :exec
INSERT INTO stock_bulk_job_items (job_id, organization_id, unit_id, status, reason, meters)
SELECT sqlc.arg(job_id), sqlc.arg(organization_id), x.unit_id, x.status, NULLIF(x.reason, ''), x.meters
FROM jsonb_to_recordset(sqlc.arg(items)::jsonb) AS x (unit_id bigint, status text, reason text, meters numeric);

-- name: GetStockBulkJob :one
SELECT * FROM stock_bulk_jobs WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: GetStockBulkJobByID :one
SELECT * FROM stock_bulk_jobs WHERE id = sqlc.arg(id);

-- name: LockStockBulkJob :one
SELECT * FROM stock_bulk_jobs WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: SetStockBulkJobStatus :one
UPDATE stock_bulk_jobs
SET status = sqlc.arg(status)::text,
    run_by = COALESCE(sqlc.narg(run_by), run_by),
    undone_by = COALESCE(sqlc.narg(undone_by), undone_by),
    started_at = CASE WHEN sqlc.arg(status)::text = 'running' AND started_at IS NULL THEN NOW() ELSE started_at END,
    finished_at = CASE WHEN sqlc.arg(status)::text IN ('completed', 'failed') THEN NOW() ELSE finished_at END,
    undone_at = CASE WHEN sqlc.arg(status)::text = 'undone' THEN NOW() ELSE undone_at END,
    error = sqlc.narg(error)
WHERE id = sqlc.arg(id) AND status = ANY (sqlc.arg(from_statuses)::text[])
RETURNING *;

-- name: RefreshStockBulkJobCounts :exec
UPDATE stock_bulk_jobs j SET
    succeeded = c.applied, failed = c.failed, reverted = c.reverted
FROM (
    SELECT count(*) FILTER (WHERE status IN ('applied', 'reverted', 'revert_skipped'))::int AS applied,
           count(*) FILTER (WHERE status = 'failed')::int AS failed,
           count(*) FILTER (WHERE status = 'reverted')::int AS reverted
    FROM stock_bulk_job_items WHERE job_id = sqlc.arg(id)
) c
WHERE j.id = sqlc.arg(id);

-- name: ListStockBulkJobItemsToRun :many
SELECT i.*, u.product_id, u.barcode
FROM stock_bulk_job_items i
JOIN units u ON u.id = i.unit_id
WHERE i.job_id = sqlc.arg(job_id) AND i.status = sqlc.arg(status)
ORDER BY i.id
LIMIT sqlc.arg(row_limit)
FOR UPDATE OF i SKIP LOCKED;

-- name: UpdateStockBulkJobItem :exec
UPDATE stock_bulk_job_items
SET status = sqlc.arg(status), reason = sqlc.narg(reason), previous = COALESCE(sqlc.narg(previous), previous),
    movement_id = COALESCE(sqlc.narg(movement_id), movement_id), error = sqlc.narg(error), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: CountStockBulkJobItemsByReason :many
SELECT status, COALESCE(reason, '')::text AS reason, count(*)::int AS n
FROM stock_bulk_job_items WHERE job_id = sqlc.arg(job_id)
GROUP BY 1, 2 ORDER BY 1, 2;

-- name: ListStockBulkJobItems :many
SELECT i.id, i.status, i.reason, i.meters, i.previous, i.error, i.updated_at,
       u.uuid AS unit_uuid, u.barcode, u.status AS unit_status, u.initial_meters, u.lot, u.note,
       p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name
FROM stock_bulk_job_items i
JOIN units u ON u.id = i.unit_id
JOIN products p ON p.id = u.product_id
WHERE i.job_id = sqlc.arg(job_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR i.status = ANY (sqlc.narg(statuses)::text[]))
ORDER BY i.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountStockBulkJobItems :one
SELECT count(*) FROM stock_bulk_job_items
WHERE job_id = sqlc.arg(job_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR status = ANY (sqlc.narg(statuses)::text[]));

-- name: ListStockBulkJobs :many
-- Sort: docs/list-contract.md, keys from stockbulk usecase JobSort.
SELECT j.*, cu.name AS created_by_name, cu.surname AS created_by_surname
FROM stock_bulk_jobs j
JOIN users cu ON cu.id = j.created_by
WHERE j.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR j.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(actions)::text[]), 0) = 0 OR j.action = ANY (sqlc.narg(actions)::text[]))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN j.created_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN j.created_at END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'eligible' THEN j.eligible END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'eligible' THEN j.eligible END
  END DESC,
  j.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountStockBulkJobs :one
SELECT count(*) FROM stock_bulk_jobs j
WHERE j.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR j.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(actions)::text[]), 0) = 0 OR j.action = ANY (sqlc.narg(actions)::text[]));

-- name: SetUnitLot :exec
UPDATE units SET lot = sqlc.narg(lot)::varchar WHERE id = sqlc.arg(id);

-- name: SetUnitNote :exec
UPDATE units SET note = sqlc.narg(note)::text WHERE id = sqlc.arg(id);

-- name: GetUnitLotNote :one
SELECT u.lot, u.note, u.product_id, u.initial_meters, u.remaining_meters,
       COALESCE(st.last_movement_id, 0)::bigint AS last_movement_id,
       COALESCE(st.owner_type, '')::text AS owner_type, COALESCE(st.owner_id, 0)::bigint AS owner_id
FROM units u LEFT JOIN unit_current_state st ON st.unit_id = u.id
WHERE u.id = sqlc.arg(id);
