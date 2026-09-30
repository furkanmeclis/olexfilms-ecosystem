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

-- name: ListExportJobsForActor :many
SELECT * FROM export_jobs
WHERE actor_id = $1
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountExportJobsForActor :one
SELECT COUNT(*)::bigint FROM export_jobs WHERE actor_id = $1;

-- name: ListAllExportJobs :many
SELECT * FROM export_jobs
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAllExportJobs :one
SELECT COUNT(*)::bigint FROM export_jobs;

-- name: ListExportJobsForOrganization :many
SELECT * FROM export_jobs
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountExportJobsForOrganization :one
SELECT COUNT(*)::bigint FROM export_jobs WHERE organization_id = $1;
