-- TEC-249 (F2-04d): short URLs behind /s/{token}.

-- name: CreateShortURL :one
-- ON CONFLICT on the token returns no row: the caller draws a new token.
INSERT INTO short_urls (organization_id, brand_id, token, target_path, expires_at, created_by)
VALUES (
    sqlc.narg(organization_id)::bigint,
    sqlc.arg(brand_id)::bigint,
    sqlc.arg(token)::varchar,
    sqlc.arg(target_path)::varchar,
    sqlc.narg(expires_at)::timestamptz,
    sqlc.narg(created_by)::bigint
)
ON CONFLICT (token) DO NOTHING
RETURNING id, uuid, token, target_path, expires_at, created_at;

-- name: HitShortURL :one
-- Resolves a live token of the brand and counts the hit in one statement.
UPDATE short_urls
SET hit_count = hit_count + 1,
    last_hit_at = sqlc.arg(now)::timestamptz
WHERE token = sqlc.arg(token)::varchar
  AND brand_id = sqlc.arg(brand_id)::bigint
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now)::timestamptz)
RETURNING target_path, expires_at;

-- name: GetShortURLExpiry :one
-- Tells an expired token of the brand apart from an unknown one.
SELECT expires_at
FROM short_urls
WHERE token = sqlc.arg(token)::varchar
  AND brand_id = sqlc.arg(brand_id)::bigint;

-- name: GetShortURLStats :one
SELECT hit_count, last_hit_at
FROM short_urls
WHERE token = sqlc.arg(token)::varchar;
