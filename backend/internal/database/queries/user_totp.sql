-- name: GetUserTOTPByUserID :one
SELECT user_id, secret_enc, enabled, confirmed_at, recovery_hashes, created_at, updated_at
FROM user_totp
WHERE user_id = $1;

-- name: UpsertUserTOTPSetup :one
INSERT INTO user_totp (user_id, secret_enc, enabled, confirmed_at, recovery_hashes)
VALUES ($1, $2, FALSE, NULL, '{}')
ON CONFLICT (user_id) DO UPDATE
SET secret_enc = EXCLUDED.secret_enc,
    enabled = FALSE,
    confirmed_at = NULL,
    recovery_hashes = '{}'
RETURNING user_id, secret_enc, enabled, confirmed_at, recovery_hashes, created_at, updated_at;

-- name: ConfirmUserTOTP :one
UPDATE user_totp
SET enabled = TRUE,
    confirmed_at = NOW(),
    recovery_hashes = $2
WHERE user_id = $1
RETURNING user_id, secret_enc, enabled, confirmed_at, recovery_hashes, created_at, updated_at;

-- name: UpdateUserTOTPRecoveryHashes :exec
UPDATE user_totp
SET recovery_hashes = $2
WHERE user_id = $1;

-- name: DeleteUserTOTP :exec
DELETE FROM user_totp
WHERE user_id = $1;
