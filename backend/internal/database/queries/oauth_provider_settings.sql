-- name: GetOAuthProviderSettings :one
SELECT * FROM oauth_provider_settings
WHERE provider = sqlc.arg(provider);

-- name: ListOAuthProviderSettings :many
SELECT * FROM oauth_provider_settings
ORDER BY provider ASC;

-- name: UpdateOAuthProviderSettings :one
UPDATE oauth_provider_settings
SET login_enabled = COALESCE(sqlc.narg(login_enabled), login_enabled),
    register_enabled = COALESCE(sqlc.narg(register_enabled), register_enabled),
    client_id = COALESCE(sqlc.narg(client_id), client_id),
    client_secret_enc = COALESCE(sqlc.narg(client_secret_enc), client_secret_enc)
WHERE provider = sqlc.arg(provider)
RETURNING *;
