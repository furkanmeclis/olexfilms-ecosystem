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

-- Phone OTP (TEC-92). Timestamps are passed in so tests can drive the clock.

-- name: LockOTPSubject :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(subject)::text, 92));

-- name: GetLatestPhoneOTP :one
SELECT *
FROM otp_codes
WHERE phone_e164 = $1
  AND type = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: CountPhoneOTPsSince :one
SELECT COUNT(*)::bigint
FROM otp_codes
WHERE phone_e164 = sqlc.arg(phone_e164)
  AND created_at >= sqlc.arg(since);

-- name: InvalidateActivePhoneOTPs :exec
UPDATE otp_codes
SET consumed_at = sqlc.arg(now)
WHERE phone_e164 = sqlc.arg(phone_e164)
  AND type = sqlc.arg(type)
  AND consumed_at IS NULL;

-- name: CreatePhoneOTP :one
INSERT INTO otp_codes (
    uuid, user_id, phone_e164, code_hash, type, expires_at, max_attempts,
    ip, user_agent, kvkk_locale, kvkk_version, message_sha256, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.narg(user_id), sqlc.arg(phone_e164), sqlc.arg(code_hash), sqlc.arg(type),
    sqlc.arg(expires_at), sqlc.arg(max_attempts), sqlc.narg(ip), sqlc.narg(user_agent),
    sqlc.narg(kvkk_locale), sqlc.narg(kvkk_version), sqlc.narg(message_sha256), sqlc.arg(created_at)
)
RETURNING *;

-- name: MarkOTPDelivered :exec
UPDATE otp_codes
SET channel = sqlc.arg(channel),
    provider_ref = sqlc.narg(provider_ref),
    delivered_at = sqlc.arg(delivered_at),
    delivery_error = sqlc.narg(delivery_error)
WHERE id = sqlc.arg(id);

-- name: MarkOTPDeliveryFailed :exec
UPDATE otp_codes
SET delivery_error = sqlc.arg(delivery_error),
    consumed_at = COALESCE(consumed_at, sqlc.arg(now))
WHERE id = sqlc.arg(id);

-- name: GetActivePhoneOTP :one
SELECT *
FROM otp_codes
WHERE phone_e164 = sqlc.arg(phone_e164)
  AND type = sqlc.arg(type)
  AND consumed_at IS NULL
  AND expires_at > sqlc.arg(now)
ORDER BY created_at DESC
LIMIT 1;

-- name: ConsumeOTPAt :exec
UPDATE otp_codes
SET consumed_at = sqlc.arg(now)
WHERE id = sqlc.arg(id)
  AND consumed_at IS NULL;

-- name: GetOTPByUUID :one
SELECT * FROM otp_codes WHERE uuid = $1;
