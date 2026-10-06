-- name: InsertActivityEvent :one
INSERT INTO activity_events (actor_user_id, action, resource, resource_uuid, payload, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListActivityEvents :many
-- Sort: docs/list-contract.md, keys from activity/usecase.SortSpec (TEC-365).
SELECT * FROM activity_events
WHERE (
    sqlc.narg(actor_uuid)::uuid IS NULL
    OR actor_user_id = (SELECT u.id FROM users u WHERE u.uuid = sqlc.narg(actor_uuid))
  )
  AND (
    COALESCE(cardinality(sqlc.narg(resources)::text[]), 0) = 0
    OR resource = ANY (sqlc.narg(resources)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(actions)::text[]), 0) = 0
    OR action = ANY (sqlc.narg(actions)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR action ILIKE '%' || sqlc.narg(q) || '%'
    OR resource ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'action' THEN action WHEN 'resource' THEN resource END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'action' THEN action WHEN 'resource' THEN resource END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountActivityEvents :one
SELECT COUNT(*)::bigint FROM activity_events
WHERE (
    sqlc.narg(actor_uuid)::uuid IS NULL
    OR actor_user_id = (SELECT u.id FROM users u WHERE u.uuid = sqlc.narg(actor_uuid))
  )
  AND (
    COALESCE(cardinality(sqlc.narg(resources)::text[]), 0) = 0
    OR resource = ANY (sqlc.narg(resources)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(actions)::text[]), 0) = 0
    OR action = ANY (sqlc.narg(actions)::text[])
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR action ILIKE '%' || sqlc.narg(q) || '%'
    OR resource ILIKE '%' || sqlc.narg(q) || '%'
  );
