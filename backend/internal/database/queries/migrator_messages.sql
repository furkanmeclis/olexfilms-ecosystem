-- TEC-263: migrator steps 10-11 (hub short_urls -> short_urls, hub
-- sms_logs / notifications -> legacy_messages). Written only by
-- cmd/migrator inside a step transaction.

-- name: MigratorShortURLByUUID :one
SELECT id, token, target_path, legacy_target_url FROM short_urls WHERE uuid = $1;

-- name: MigratorShortURLTokenTaken :one
SELECT EXISTS (SELECT 1 FROM short_urls WHERE token = sqlc.arg(token)::varchar)::boolean AS taken;

-- name: MigratorInsertShortURL :one
-- A legacy token is kept as is (old links keep resolving); migrated links
-- are brand-wide (no organization) and never expire, like the hub's.
INSERT INTO short_urls (uuid, brand_id, token, target_path, legacy_target_url, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(brand_id)::bigint, sqlc.arg(token)::varchar, sqlc.arg(target_path)::varchar,
    sqlc.narg(legacy_target_url)::varchar, COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
ON CONFLICT (token) DO NOTHING
RETURNING id;

-- name: MigratorUpdateShortURL :exec
UPDATE short_urls
SET target_path       = sqlc.arg(target_path)::varchar,
    legacy_target_url = sqlc.narg(legacy_target_url)::varchar
WHERE id = sqlc.arg(id)::bigint;

-- name: MigratorInsertLegacyMessage :one
-- legacy_messages is append-only: a row already imported is left alone.
INSERT INTO legacy_messages (
    uuid, organization_id, brand_id, channel, recipient, user_id, body, payload, sent_at,
    source_system, source_table, source_id
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint, sqlc.arg(channel)::varchar,
    sqlc.narg(recipient)::varchar, sqlc.narg(user_id)::bigint, sqlc.arg(body)::text, sqlc.arg(payload)::jsonb,
    sqlc.narg(sent_at)::timestamptz, sqlc.arg(source_system)::varchar, sqlc.arg(source_table)::varchar,
    sqlc.arg(source_id)::varchar
)
ON CONFLICT (source_system, source_table, source_id) DO NOTHING
RETURNING id;

-- name: MigratorLegacyMessageExists :one
SELECT EXISTS (
    SELECT 1 FROM legacy_messages
    WHERE source_system = sqlc.arg(source_system)::varchar
      AND source_table = sqlc.arg(source_table)::varchar
      AND source_id = sqlc.arg(source_id)::varchar
)::boolean AS present;
