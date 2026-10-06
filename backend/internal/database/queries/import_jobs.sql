-- name: CreateImportJob :one
INSERT INTO import_jobs (resource, actor_id, format, locale, status, file_key, organization_id, source_filename)
VALUES ($1, $2, $3, $4, 'uploaded', $5, sqlc.narg(organization_id), sqlc.arg(source_filename))
RETURNING *;

-- name: UpdateImportJobFileKey :one
UPDATE import_jobs
SET file_key = $2
WHERE uuid = $1
RETURNING *;

-- name: GetImportJobByUUID :one
SELECT * FROM import_jobs WHERE uuid = $1;

-- name: GetImportJobByID :one
SELECT * FROM import_jobs WHERE id = $1;

-- name: UpdateImportJobMapping :one
UPDATE import_jobs
SET mapping_json = $2,
    defaults_json = $3,
    status = 'mapped'
WHERE uuid = $1 AND status IN ('uploaded', 'mapped', 'previewed')
RETURNING *;

-- name: UpdateImportJobPreview :one
UPDATE import_jobs
SET preview_json = $2,
    status = 'previewed'
WHERE uuid = $1 AND status IN ('mapped', 'previewed')
RETURNING *;

-- name: QueueImportJob :one
UPDATE import_jobs
SET status = 'queued'
WHERE uuid = $1 AND status = 'previewed'
RETURNING *;

-- name: MarkImportJobApplying :one
UPDATE import_jobs
SET status = 'applying'
WHERE id = $1 AND status = 'queued'
RETURNING *;

-- name: MarkImportJobApplied :one
UPDATE import_jobs
SET status = 'applied',
    applied_at = NOW(),
    rollback_until = NOW() + INTERVAL '24 hours',
    preview_json = $2
WHERE id = $1
RETURNING *;

-- name: MarkImportJobFailed :one
UPDATE import_jobs
SET status = 'failed',
    error = $2
WHERE id = $1
RETURNING *;

-- name: SetImportJobPreview :one
-- TEC-158: staged importers keep their apply/undo report in preview_json.
UPDATE import_jobs
SET preview_json = $2
WHERE id = $1
RETURNING *;

-- name: MarkImportJobRolledBack :one
UPDATE import_jobs
SET status = 'rolled_back'
WHERE id = $1 AND status = 'applied'
RETURNING *;

-- name: ListImportJobsFiltered :many
-- TEC-365: platform jobs (organization_id NULL; actor_id = own jobs, or NULL
-- for admins) and tenant jobs (organization_id). Sort:
-- docs/list-contract.md, keys from imports/usecase.SortSpec.
SELECT sqlc.embed(i), u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname
FROM import_jobs i
JOIN users u ON u.id = i.actor_id
WHERE (
    (sqlc.narg(organization_id)::bigint IS NULL AND i.organization_id IS NULL)
    OR i.organization_id = sqlc.narg(organization_id)
  )
  AND (sqlc.narg(actor_id)::bigint IS NULL OR i.actor_id = sqlc.narg(actor_id))
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR i.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(resources)::text[]), 0) = 0
    OR i.resource = ANY (sqlc.narg(resources)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(formats)::text[]), 0) = 0
    OR i.format = ANY (sqlc.narg(formats)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR i.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR i.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR i.resource ILIKE '%' || sqlc.narg(q) || '%'
    OR i.source_filename ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN i.status WHEN 'resource' THEN i.resource WHEN 'format' THEN i.format END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN i.status WHEN 'resource' THEN i.resource WHEN 'format' THEN i.format END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN i.created_at WHEN 'updated_at' THEN i.updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN i.created_at WHEN 'updated_at' THEN i.updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN i.id END DESC,
  i.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountImportJobsFiltered :one
SELECT COUNT(*)::bigint
FROM import_jobs i
WHERE (
    (sqlc.narg(organization_id)::bigint IS NULL AND i.organization_id IS NULL)
    OR i.organization_id = sqlc.narg(organization_id)
  )
  AND (sqlc.narg(actor_id)::bigint IS NULL OR i.actor_id = sqlc.narg(actor_id))
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR i.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(resources)::text[]), 0) = 0
    OR i.resource = ANY (sqlc.narg(resources)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(formats)::text[]), 0) = 0
    OR i.format = ANY (sqlc.narg(formats)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR i.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR i.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR i.resource ILIKE '%' || sqlc.narg(q) || '%'
    OR i.source_filename ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: InsertImportChange :one
INSERT INTO import_changes (job_id, entity_type, entity_uuid, op, previous_json)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListImportChangesForJob :many
SELECT * FROM import_changes
WHERE job_id = $1
ORDER BY id ASC;
