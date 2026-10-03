-- TEC-273 (F2-02h): Glorian admin API. Every read is limited to one
-- connection; the handler resolves the connection of the glorian brand
-- first.

-- name: ListGlorianSyncRuns :many
-- Sync runs of a connection, newest first, optionally filtered by kind
-- and status.
SELECT * FROM integration_sync_runs
WHERE connection_id = sqlc.arg(connection_id)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY started_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetGlorianSyncRunByUUID :one
SELECT * FROM integration_sync_runs
WHERE uuid = sqlc.arg(uuid) AND connection_id = sqlc.arg(connection_id);

-- name: GetIntegrationSyncRunByID :one
SELECT * FROM integration_sync_runs
WHERE id = sqlc.arg(id);

-- name: ListGlorianOutbounds :many
-- Order outbounds of a connection in one state with their order, oldest
-- first (the replay order).
SELECT ob.id, ob.uuid, ob.order_id, ob.external_reference, ob.state, ob.held_reason,
       ob.attempts, ob.last_error, ob.created_at, ob.updated_at,
       o.uuid AS order_uuid, o.order_no, o.status AS order_status
FROM order_outbounds ob
JOIN orders o ON o.id = ob.order_id
WHERE ob.connection_id = sqlc.arg(connection_id)
  AND ob.state = sqlc.arg(state)
ORDER BY ob.id
LIMIT sqlc.arg(row_limit);

-- name: GetGlorianOutboundByUUID :one
SELECT ob.id, ob.uuid, ob.order_id, ob.external_reference, ob.state, ob.held_reason,
       ob.attempts, ob.last_error, ob.created_at, ob.updated_at,
       o.uuid AS order_uuid, o.order_no, o.status AS order_status
FROM order_outbounds ob
JOIN orders o ON o.id = ob.order_id
WHERE ob.uuid = sqlc.arg(uuid) AND ob.connection_id = sqlc.arg(connection_id);
