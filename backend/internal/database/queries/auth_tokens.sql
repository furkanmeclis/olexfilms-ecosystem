-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (
    user_id, token_hash, expires_at, user_agent, ip_address, impersonator_user_id, organization_id, realm,
    client, device_id, device_name, platform, app_version, family_id
)
VALUES (
    sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at), sqlc.narg(user_agent), sqlc.narg(ip_address),
    sqlc.narg(impersonator_user_id), sqlc.narg(organization_id), sqlc.arg(realm),
    sqlc.arg(client), sqlc.narg(device_id), sqlc.narg(device_name), sqlc.narg(platform), sqlc.narg(app_version),
    sqlc.narg(family_id)
)
RETURNING *;

-- name: GetValidRefreshTokenByHash :one
SELECT *
FROM refresh_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: RevokeRefreshTokenByHash :execrows
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE token_hash = $1
  AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokensForUser :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE user_id = $1
  AND revoked_at IS NULL;

-- name: ListActiveRefreshTokensByUserID :many
SELECT *
FROM refresh_tokens
WHERE user_id = $1
  AND revoked_at IS NULL
  AND expires_at > NOW()
ORDER BY created_at DESC;

-- name: RevokeRefreshTokenByUUIDForUser :execrows
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE uuid = $1
  AND user_id = $2
  AND revoked_at IS NULL;

-- name: RevokeOtherRefreshTokensForUser :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE user_id = $1
  AND uuid <> $2
  AND revoked_at IS NULL;

-- TEC-91: mobile refresh chains (rotation, reuse detection, device sign-out).

-- name: RotateRefreshTokenByHash :execrows
UPDATE refresh_tokens
SET revoked_at = NOW(), rotated_at = NOW()
WHERE token_hash = $1
  AND revoked_at IS NULL;

-- name: GetRefreshTokenByHashAny :one
SELECT *
FROM refresh_tokens
WHERE token_hash = $1;

-- name: GetRefreshTokenByUUID :one
SELECT *
FROM refresh_tokens
WHERE uuid = $1;

-- name: ListActiveRefreshTokenUUIDsByFamily :many
SELECT uuid
FROM refresh_tokens
WHERE family_id = $1
  AND revoked_at IS NULL;

-- name: RevokeRefreshTokenFamily :execrows
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE family_id = $1
  AND revoked_at IS NULL;

-- name: ListActiveMobileSessionUUIDsForDevice :many
SELECT uuid
FROM refresh_tokens
WHERE user_id = $1
  AND client = 'mobile'
  AND device_id = $2
  AND uuid <> $3
  AND revoked_at IS NULL;

-- name: RevokeMobileSessionsForDevice :execrows
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE user_id = $1
  AND client = 'mobile'
  AND device_id = $2
  AND uuid <> $3
  AND revoked_at IS NULL;
