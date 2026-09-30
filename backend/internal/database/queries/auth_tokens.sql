-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address, impersonator_user_id, organization_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
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
