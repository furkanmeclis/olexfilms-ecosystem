-- name: GetGitHubAppSettings :one
SELECT * FROM github_app_settings WHERE id = 1;

-- name: UpdateGitHubAppSettings :one
UPDATE github_app_settings
SET enabled = COALESCE(sqlc.narg(enabled), enabled),
    register_enabled = COALESCE(sqlc.narg(register_enabled), register_enabled),
    app_id = COALESCE(sqlc.narg(app_id), app_id),
    client_id = COALESCE(sqlc.narg(client_id), client_id),
    client_secret_enc = COALESCE(sqlc.narg(client_secret_enc), client_secret_enc),
    private_key_enc = COALESCE(sqlc.narg(private_key_enc), private_key_enc)
WHERE id = 1
RETURNING *;

-- name: GetOAuthAccountByProviderAccount :one
SELECT oa.*, u.uuid AS user_uuid
FROM oauth_accounts oa
JOIN users u ON u.id = oa.user_id
WHERE oa.provider = sqlc.arg(provider)
  AND oa.provider_account_id = sqlc.arg(provider_account_id)
  AND u.deleted_at IS NULL;

-- name: GetOAuthAccountByUserProvider :one
SELECT oa.*, u.uuid AS user_uuid
FROM oauth_accounts oa
JOIN users u ON u.id = oa.user_id
WHERE oa.user_id = sqlc.arg(user_id)
  AND oa.provider = sqlc.arg(provider)
  AND u.deleted_at IS NULL;

-- name: ListOAuthAccountsByUserID :many
SELECT oa.*, u.uuid AS user_uuid
FROM oauth_accounts oa
JOIN users u ON u.id = oa.user_id
WHERE oa.user_id = sqlc.arg(user_id)
  AND u.deleted_at IS NULL
ORDER BY oa.created_at ASC;

-- name: ListOAuthAccountsForUserIDs :many
SELECT oa.*, u.uuid AS user_uuid
FROM oauth_accounts oa
JOIN users u ON u.id = oa.user_id
WHERE oa.user_id = ANY (sqlc.arg(user_ids)::bigint[])
  AND u.deleted_at IS NULL
ORDER BY oa.user_id, oa.created_at ASC;

-- name: CreateOAuthAccount :one
INSERT INTO oauth_accounts (
    user_id,
    provider,
    provider_account_id,
    type,
    access_token_enc,
    refresh_token_enc,
    expires_at,
    token_type,
    scope,
    github_login
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(provider),
    sqlc.arg(provider_account_id),
    sqlc.arg(type),
    sqlc.narg(access_token_enc),
    sqlc.narg(refresh_token_enc),
    sqlc.narg(expires_at),
    sqlc.narg(token_type),
    sqlc.narg(scope),
    sqlc.narg(github_login)
)
RETURNING *;

-- name: DeleteOAuthAccountByUserProvider :exec
DELETE FROM oauth_accounts
WHERE user_id = sqlc.arg(user_id)
  AND provider = sqlc.arg(provider);

-- name: DeleteOAuthAccountByProviderAccount :exec
DELETE FROM oauth_accounts
WHERE provider = sqlc.arg(provider)
  AND provider_account_id = sqlc.arg(provider_account_id);
