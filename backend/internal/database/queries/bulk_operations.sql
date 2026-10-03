-- TEC-212: bulk operation log + undo.

-- name: InsertBulkOperation :one
INSERT INTO bulk_operations (
    organization_id, brand_id, job_id, resource, action, permission, actor_user_id,
    target_json, changes, total, succeeded, failed, undo_status, undo_until
)
VALUES (
    sqlc.narg(organization_id), sqlc.narg(brand_id), sqlc.narg(job_id),
    sqlc.arg(resource), sqlc.arg(action), sqlc.arg(permission), sqlc.arg(actor_user_id),
    sqlc.arg(target_json), sqlc.arg(changes), sqlc.arg(total), sqlc.arg(succeeded), sqlc.arg(failed),
    sqlc.arg(undo_status), sqlc.narg(undo_until)
)
RETURNING *;

-- An operation visible from the given scope: a tenant operation of that
-- organization, or (organization_id NULL in the query) a platform operation.
-- name: GetBulkOperationByUUID :one
SELECT * FROM bulk_operations
WHERE uuid = sqlc.arg(uuid)
  AND organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)::bigint;

-- name: GetBulkOperationByJobID :one
SELECT * FROM bulk_operations WHERE job_id = sqlc.arg(job_id);

-- name: LockBulkOperationForUndo :one
SELECT * FROM bulk_operations
WHERE id = sqlc.arg(id) AND undo_status = 'available'
FOR UPDATE;

-- name: MarkBulkOperationUndone :one
UPDATE bulk_operations
SET undo_status = sqlc.arg(undo_status),
    undone_at = NOW(),
    undone_by_user_id = sqlc.arg(undone_by_user_id),
    undo_result = sqlc.arg(undo_result)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListBulkOperationsForOrganization :many
SELECT * FROM bulk_operations
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountBulkOperationsForOrganization :one
SELECT COUNT(*)::bigint FROM bulk_operations WHERE organization_id = sqlc.arg(organization_id);
