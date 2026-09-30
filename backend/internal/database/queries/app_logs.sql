-- name: InsertAppLog :exec
INSERT INTO app_logs (level, message, source, attrs, request_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetAppLogByUUID :one
SELECT * FROM app_logs
WHERE uuid = $1;

-- name: ListAppLogs :many
SELECT * FROM app_logs
WHERE (
        sqlc.narg(levels)::text[] IS NULL
        OR cardinality(sqlc.narg(levels)::text[]) = 0
        OR level = ANY (sqlc.narg(levels)::text[])
    )
  AND (sqlc.narg(source)::text IS NULL OR source = sqlc.narg(source))
  AND (
        sqlc.narg(q)::text IS NULL
        OR message ILIKE '%' || sqlc.narg(q) || '%'
        OR source ILIKE '%' || sqlc.narg(q) || '%'
        OR COALESCE(request_id, '') ILIKE '%' || sqlc.narg(q) || '%'
    )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at < sqlc.narg(created_to))
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAppLogs :one
SELECT COUNT(*)::bigint FROM app_logs
WHERE (
        sqlc.narg(levels)::text[] IS NULL
        OR cardinality(sqlc.narg(levels)::text[]) = 0
        OR level = ANY (sqlc.narg(levels)::text[])
    )
  AND (sqlc.narg(source)::text IS NULL OR source = sqlc.narg(source))
  AND (
        sqlc.narg(q)::text IS NULL
        OR message ILIKE '%' || sqlc.narg(q) || '%'
        OR source ILIKE '%' || sqlc.narg(q) || '%'
        OR COALESCE(request_id, '') ILIKE '%' || sqlc.narg(q) || '%'
    )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at < sqlc.narg(created_to));

-- name: CountAppLogsByLevel :many
SELECT level, COUNT(*)::bigint AS count
FROM app_logs
GROUP BY level;

-- name: ListAppLogSources :many
SELECT DISTINCT source
FROM app_logs
WHERE source <> ''
ORDER BY source
LIMIT 200;

-- name: DeleteAppLogByUUID :execrows
DELETE FROM app_logs
WHERE uuid = $1;

-- name: DeleteAppLogsByUUIDs :execrows
DELETE FROM app_logs
WHERE uuid = ANY (sqlc.arg(uuids)::uuid[]);

-- name: DeleteAppLogsMatching :execrows
DELETE FROM app_logs
WHERE (
        sqlc.narg(older_than_hours)::int IS NULL
        OR created_at < NOW() - make_interval(hours => sqlc.narg(older_than_hours)::int)
    )
  AND (
        sqlc.narg(levels)::text[] IS NULL
        OR cardinality(sqlc.narg(levels)::text[]) = 0
        OR level = ANY (sqlc.narg(levels)::text[])
    )
  AND (sqlc.narg(source)::text IS NULL OR source = sqlc.narg(source))
  AND (
        sqlc.narg(q)::text IS NULL
        OR message ILIKE '%' || sqlc.narg(q) || '%'
        OR source ILIKE '%' || sqlc.narg(q) || '%'
    )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at < sqlc.narg(created_to));

-- name: ListLogPurgeRules :many
SELECT * FROM log_purge_rules
ORDER BY is_system DESC, name ASC;

-- name: GetLogPurgeRuleByUUID :one
SELECT * FROM log_purge_rules
WHERE uuid = $1;

-- name: ListEnabledLogPurgeRules :many
SELECT * FROM log_purge_rules
WHERE enabled = TRUE
ORDER BY id ASC;

-- name: CreateLogPurgeRule :one
INSERT INTO log_purge_rules (
    name, enabled, is_system, levels, source, message_contains,
    older_than_hours, interval_minutes, created_by
)
VALUES ($1, $2, FALSE, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateLogPurgeRule :one
UPDATE log_purge_rules
SET name = sqlc.arg(name),
    enabled = sqlc.arg(enabled),
    levels = sqlc.arg(levels),
    source = sqlc.narg(source),
    message_contains = sqlc.narg(message_contains),
    older_than_hours = sqlc.arg(older_than_hours),
    interval_minutes = sqlc.arg(interval_minutes)
WHERE uuid = sqlc.arg(uuid)
RETURNING *;

-- name: DeleteLogPurgeRule :execrows
DELETE FROM log_purge_rules
WHERE uuid = $1
  AND is_system = FALSE;

-- name: MarkLogPurgeRuleRun :one
UPDATE log_purge_rules
SET last_run_at = NOW(),
    last_deleted_count = sqlc.arg(deleted_count),
    last_error = sqlc.narg(last_error)
WHERE uuid = sqlc.arg(uuid)
RETURNING *;
