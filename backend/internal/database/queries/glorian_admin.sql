-- TEC-273 (F2-02h): Glorian admin API. Every read is limited to one
-- connection; the handler resolves the connection of the glorian brand
-- first.

-- name: ListGlorianSyncRuns :many
-- Sync runs of a connection (TEC-367 list contract, keys from
-- usecase.SyncRunsSortSpec; default -started_at).
SELECT * FROM integration_sync_runs
WHERE connection_id = sqlc.arg(connection_id)
  AND (
    COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
    OR kind = ANY (sqlc.narg(kinds)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (sqlc.narg(started_from)::timestamptz IS NULL OR started_at >= sqlc.narg(started_from))
  AND (sqlc.narg(started_before)::timestamptz IS NULL OR started_at < sqlc.narg(started_before))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'kind' THEN kind::text WHEN 'status' THEN status::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'kind' THEN kind::text WHEN 'status' THEN status::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'started_at' THEN started_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'started_at' THEN started_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'finished_at' THEN finished_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'finished_at' THEN finished_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGlorianSyncRuns :one
SELECT COUNT(*)::bigint FROM integration_sync_runs
WHERE connection_id = sqlc.arg(connection_id)
  AND (
    COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
    OR kind = ANY (sqlc.narg(kinds)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (sqlc.narg(started_from)::timestamptz IS NULL OR started_at >= sqlc.narg(started_from))
  AND (sqlc.narg(started_before)::timestamptz IS NULL OR started_at < sqlc.narg(started_before));

-- name: GetGlorianSyncRunByUUID :one
SELECT * FROM integration_sync_runs
WHERE uuid = sqlc.arg(uuid) AND connection_id = sqlc.arg(connection_id);

-- name: GetIntegrationSyncRunByID :one
SELECT * FROM integration_sync_runs
WHERE id = sqlc.arg(id);

-- name: ListGlorianOutbounds :many
-- Order outbounds of a connection with their order (TEC-367 list contract,
-- keys from usecase.OutboundsSortSpec; default created_at, the replay order).
SELECT ob.id, ob.uuid, ob.order_id, ob.external_reference, ob.state, ob.held_reason,
       ob.attempts, ob.last_error, ob.created_at, ob.updated_at,
       o.uuid AS order_uuid, o.order_no, o.status AS order_status
FROM order_outbounds ob
JOIN orders o ON o.id = ob.order_id
WHERE ob.connection_id = sqlc.arg(connection_id)
  AND (
    COALESCE(cardinality(sqlc.narg(states)::text[]), 0) = 0
    OR ob.state = ANY (sqlc.narg(states)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR o.order_no ILIKE '%' || sqlc.narg(q) || '%'
    OR ob.external_reference ILIKE '%' || sqlc.narg(q) || '%'
  )
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR ob.updated_at >= sqlc.narg(updated_from))
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR ob.updated_at < sqlc.narg(updated_before))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'state' THEN ob.state::text WHEN 'order_no' THEN o.order_no::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'state' THEN ob.state::text WHEN 'order_no' THEN o.order_no::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'attempts' THEN ob.attempts END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'attempts' THEN ob.attempts END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN ob.created_at WHEN 'updated_at' THEN ob.updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN ob.created_at WHEN 'updated_at' THEN ob.updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN ob.id END DESC,
  ob.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGlorianOutbounds :one
SELECT COUNT(*)::bigint
FROM order_outbounds ob
JOIN orders o ON o.id = ob.order_id
WHERE ob.connection_id = sqlc.arg(connection_id)
  AND (
    COALESCE(cardinality(sqlc.narg(states)::text[]), 0) = 0
    OR ob.state = ANY (sqlc.narg(states)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR o.order_no ILIKE '%' || sqlc.narg(q) || '%'
    OR ob.external_reference ILIKE '%' || sqlc.narg(q) || '%'
  )
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR ob.updated_at >= sqlc.narg(updated_from))
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR ob.updated_at < sqlc.narg(updated_before));

-- name: GetGlorianOutboundByUUID :one
SELECT ob.id, ob.uuid, ob.order_id, ob.external_reference, ob.state, ob.held_reason,
       ob.attempts, ob.last_error, ob.created_at, ob.updated_at,
       o.uuid AS order_uuid, o.order_no, o.status AS order_status
FROM order_outbounds ob
JOIN orders o ON o.id = ob.order_id
WHERE ob.uuid = sqlc.arg(uuid) AND ob.connection_id = sqlc.arg(connection_id);
