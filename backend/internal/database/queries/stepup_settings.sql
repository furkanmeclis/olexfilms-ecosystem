-- name: GetStepupSettings :one
SELECT id, ttl_hours, password_enabled, passkey_enabled, totp_enabled, password_login_totp_required, created_at, updated_at
FROM stepup_settings
WHERE id = 1;

-- name: UpdateStepupSettings :one
UPDATE stepup_settings
SET ttl_hours = COALESCE(sqlc.narg(ttl_hours), ttl_hours),
    password_enabled = COALESCE(sqlc.narg(password_enabled), password_enabled),
    passkey_enabled = COALESCE(sqlc.narg(passkey_enabled), passkey_enabled),
    totp_enabled = COALESCE(sqlc.narg(totp_enabled), totp_enabled),
    password_login_totp_required = COALESCE(sqlc.narg(password_login_totp_required), password_login_totp_required)
WHERE id = 1
RETURNING id, ttl_hours, password_enabled, passkey_enabled, totp_enabled, password_login_totp_required, created_at, updated_at;
