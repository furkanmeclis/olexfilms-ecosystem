-- name: CreateWebAuthnCredential :one
INSERT INTO webauthn_credentials (
    user_id,
    credential_id,
    public_key,
    counter,
    device_type,
    backed_up,
    transports,
    provider_account_id,
    name
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetWebAuthnCredentialByCredentialID :one
SELECT *
FROM webauthn_credentials
WHERE credential_id = $1;

-- name: ListWebAuthnCredentialsByUserID :many
SELECT *
FROM webauthn_credentials
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListWebAuthnCredentialsForUserIDs :many
SELECT *
FROM webauthn_credentials
WHERE user_id = ANY (sqlc.arg(user_ids)::bigint[])
ORDER BY user_id, created_at ASC;

-- name: GetWebAuthnCredentialByUUID :one
SELECT wc.*
FROM webauthn_credentials wc
INNER JOIN users u ON u.id = wc.user_id
WHERE wc.uuid = $1
  AND u.uuid = $2
  AND u.deleted_at IS NULL;

-- name: UpdateWebAuthnCredentialCounter :exec
UPDATE webauthn_credentials
SET counter = $2,
    last_used_at = NOW()
WHERE credential_id = $1;

-- name: UpdateWebAuthnCredentialName :one
UPDATE webauthn_credentials wc
SET name = $3
FROM users u
WHERE wc.uuid = $1
  AND wc.user_id = u.id
  AND u.uuid = $2
  AND u.deleted_at IS NULL
RETURNING wc.*;

-- name: DeleteWebAuthnCredentialByCredentialID :exec
DELETE FROM webauthn_credentials
WHERE credential_id = $1;

-- name: DeleteWebAuthnCredentialByUUID :exec
DELETE FROM webauthn_credentials wc
USING users u
WHERE wc.uuid = $1
  AND wc.user_id = u.id
  AND u.uuid = $2
  AND u.deleted_at IS NULL;
