-- name: CreateExportJob :one
INSERT INTO export_jobs (resource, actor_id, organization_id, format, query_json, locale, status, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, 'queued', $7)
RETURNING *;

-- name: GetExportJobByUUID :one
SELECT * FROM export_jobs WHERE uuid = $1;

-- name: GetExportJobByID :one
SELECT * FROM export_jobs WHERE id = $1;

-- name: MarkExportJobProcessing :one
UPDATE export_jobs
SET status = 'processing'
WHERE id = $1 AND status = 'queued'
RETURNING *;

-- name: MarkExportJobCompleted :one
UPDATE export_jobs
SET status = 'completed',
    file_key = $2,
    row_count = $3
WHERE id = $1
RETURNING *;

-- name: MarkExportJobFailed :one
UPDATE export_jobs
SET status = 'failed',
    error = $2
WHERE id = $1
RETURNING *;

-- name: ListExportJobsFiltered :many
-- TEC-365: platform (actor_id = own jobs, or NULL for admins) and tenant
-- (organization_id) export lists. Sort: docs/list-contract.md, keys from
-- exports/usecase.SortSpec.
SELECT sqlc.embed(e), u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname
FROM export_jobs e
JOIN users u ON u.id = e.actor_id
WHERE (sqlc.narg(organization_id)::bigint IS NULL OR e.organization_id = sqlc.narg(organization_id))
  AND (sqlc.narg(actor_id)::bigint IS NULL OR e.actor_id = sqlc.narg(actor_id))
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR e.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(resources)::text[]), 0) = 0
    OR e.resource = ANY (sqlc.narg(resources)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(formats)::text[]), 0) = 0
    OR e.format = ANY (sqlc.narg(formats)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.resource ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN e.status WHEN 'resource' THEN e.resource WHEN 'format' THEN e.format END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN e.status WHEN 'resource' THEN e.resource WHEN 'format' THEN e.format END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN e.created_at WHEN 'updated_at' THEN e.updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN e.created_at WHEN 'updated_at' THEN e.updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN e.id END DESC,
  e.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountExportJobsFiltered :one
SELECT COUNT(*)::bigint
FROM export_jobs e
WHERE (sqlc.narg(organization_id)::bigint IS NULL OR e.organization_id = sqlc.narg(organization_id))
  AND (sqlc.narg(actor_id)::bigint IS NULL OR e.actor_id = sqlc.narg(actor_id))
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR e.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(resources)::text[]), 0) = 0
    OR e.resource = ANY (sqlc.narg(resources)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(formats)::text[]), 0) = 0
    OR e.format = ANY (sqlc.narg(formats)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.resource ILIKE '%' || sqlc.narg(q) || '%'
  );

-- TEC-239: the newest reusable portal job of the actor for one service
-- (no organization): a completed job created at or after not_before, or a
-- queued / processing one created at or after pending_after. Failed and
-- expired jobs are never reused.
-- name: GetReusablePortalServiceJob :one
SELECT * FROM export_jobs
WHERE actor_id = sqlc.arg(actor_id)
  AND resource = sqlc.arg(resource)::text
  AND organization_id IS NULL
  AND format = 'pdf'
  AND locale = sqlc.arg(locale)::text
  AND query_json->>'service_uuid' = sqlc.arg(service_uuid)::text
  AND (expires_at IS NULL OR expires_at > NOW())
  AND (
    (status = 'completed' AND created_at >= sqlc.arg(not_before)::timestamptz)
    OR (status IN ('queued', 'processing') AND created_at >= sqlc.arg(pending_after)::timestamptz)
  )
ORDER BY created_at DESC
LIMIT 1;
