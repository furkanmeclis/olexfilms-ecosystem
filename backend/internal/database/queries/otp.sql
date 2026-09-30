-- name: CreateOTPCode :one
INSERT INTO otp_codes (user_id, email, code_hash, type, expires_at, max_attempts)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetActiveOTPByEmailType :one
SELECT *
FROM otp_codes
WHERE email = $1
  AND type = $2
  AND consumed_at IS NULL
  AND expires_at > NOW()
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementOTPAttempts :one
UPDATE otp_codes
SET attempt_count = attempt_count + 1
WHERE id = $1
RETURNING *;

-- name: ConsumeOTP :exec
UPDATE otp_codes
SET consumed_at = NOW()
WHERE id = $1
  AND consumed_at IS NULL;

-- name: InvalidateActiveOTPs :exec
UPDATE otp_codes
SET consumed_at = NOW()
WHERE email = $1
  AND type = $2
  AND consumed_at IS NULL;
