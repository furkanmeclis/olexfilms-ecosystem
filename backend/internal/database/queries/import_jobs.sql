-- name: CreateImportJob :one
INSERT INTO import_jobs (resource, actor_id, format, locale, status, file_key, organization_id)
VALUES ($1, $2, $3, $4, 'uploaded', $5, sqlc.narg(organization_id))
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

-- name: MarkImportJobRolledBack :one
UPDATE import_jobs
SET status = 'rolled_back'
WHERE id = $1 AND status = 'applied'
RETURNING *;

-- name: ListImportJobsForActor :many
SELECT * FROM import_jobs
WHERE actor_id = $1 AND organization_id IS NULL
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountImportJobsForActor :one
SELECT COUNT(*)::bigint FROM import_jobs WHERE actor_id = $1 AND organization_id IS NULL;

-- name: ListAllImportJobs :many
SELECT * FROM import_jobs
WHERE organization_id IS NULL
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAllImportJobs :one
SELECT COUNT(*)::bigint FROM import_jobs WHERE organization_id IS NULL;

-- name: ListImportJobsForOrganization :many
SELECT * FROM import_jobs
WHERE organization_id = sqlc.arg(organization_id)::bigint
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountImportJobsForOrganization :one
SELECT COUNT(*)::bigint FROM import_jobs WHERE organization_id = sqlc.arg(organization_id)::bigint;

-- name: InsertImportChange :one
INSERT INTO import_changes (job_id, entity_type, entity_uuid, op, previous_json)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListImportChangesForJob :many
SELECT * FROM import_changes
WHERE job_id = $1
ORDER BY id ASC;
