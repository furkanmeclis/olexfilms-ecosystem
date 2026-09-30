-- name: GetAuthSettings :one
SELECT
    s.*,
    r.uuid AS default_role_uuid,
    r.name AS default_role_name,
    r.slug AS default_role_slug
FROM auth_settings s
LEFT JOIN roles r ON r.id = s.default_role_id
WHERE s.id = 1;

-- name: UpdateAuthSettings :one
UPDATE auth_settings
SET registration_enabled = sqlc.arg(registration_enabled),
    default_role_id = sqlc.narg(default_role_id),
    password_login_enabled = sqlc.arg(password_login_enabled),
    password_register_enabled = sqlc.arg(password_register_enabled),
    passkey_login_enabled = sqlc.arg(passkey_login_enabled)
WHERE id = 1
RETURNING *;
