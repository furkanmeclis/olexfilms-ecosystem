-- name: CreateBulkJob :one
INSERT INTO bulk_jobs (resource, action, actor_id, locale, status, target_json)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBulkJobByUUID :one
SELECT * FROM bulk_jobs WHERE uuid = $1;

-- name: GetBulkJobByID :one
SELECT * FROM bulk_jobs WHERE id = $1;

-- name: MarkBulkJobProcessing :one
UPDATE bulk_jobs
SET status = 'processing'
WHERE id = $1 AND status = 'queued'
RETURNING *;

-- name: MarkBulkJobCompleted :one
UPDATE bulk_jobs
SET status = 'completed',
    applied_at = NOW(),
    rollback_until = sqlc.narg(rollback_until),
    result_json = $2
WHERE id = $1
RETURNING *;

-- name: MarkBulkJobFailed :one
UPDATE bulk_jobs
SET status = 'failed',
    error = $2
WHERE id = $1
RETURNING *;

-- name: MarkBulkJobRolledBack :one
UPDATE bulk_jobs
SET status = $2
WHERE id = $1 AND status = 'completed'
RETURNING *;

-- name: ListBulkJobsForActor :many
SELECT * FROM bulk_jobs
WHERE actor_id = $1
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountBulkJobsForActor :one
SELECT COUNT(*)::bigint FROM bulk_jobs WHERE actor_id = $1;

-- name: ListAllBulkJobs :many
SELECT * FROM bulk_jobs
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAllBulkJobs :one
SELECT COUNT(*)::bigint FROM bulk_jobs;

-- name: InsertBulkChange :one
INSERT INTO bulk_changes (job_id, entity_type, entity_uuid, op, previous_json)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListBulkChangesForJob :many
SELECT * FROM bulk_changes
WHERE job_id = $1
ORDER BY id ASC;
