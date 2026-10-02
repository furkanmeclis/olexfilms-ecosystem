-- name: ListSystemSettings :many
SELECT * FROM system_settings ORDER BY key;

-- name: GetSystemSetting :one
SELECT * FROM system_settings WHERE key = $1;

-- name: UpsertSystemSetting :one
INSERT INTO system_settings (key, value, schema_version, updated_by)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(schema_version), sqlc.narg(updated_by))
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    schema_version = EXCLUDED.schema_version,
    updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: DeleteSystemSetting :execrows
DELETE FROM system_settings WHERE key = $1;
