-- TEC-400 (F4-03a): MCP OAuth 2.1 authorization server.

-- name: CreateOAuthClient :one
INSERT INTO oauth_clients (client_id, client_name, redirect_uris, created_ip)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetOAuthClient :one
SELECT * FROM oauth_clients WHERE client_id = $1;

-- name: TouchOAuthClient :exec
UPDATE oauth_clients SET last_used_at = NOW() WHERE client_id = $1;

-- name: CreateOAuthAuthRequest :one
INSERT INTO oauth_auth_requests (client_id, redirect_uri, code_challenge, state, scopes, resource, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetOAuthAuthRequest :one
-- A pending request that has not expired, with its client.
SELECT r.*, c.client_name
FROM oauth_auth_requests r
JOIN oauth_clients c ON c.client_id = r.client_id
WHERE r.id = $1 AND r.expires_at > NOW() AND c.revoked_at IS NULL;

-- name: DeleteOAuthAuthRequest :execrows
-- Deleting is the claim: a concurrent second decision finds nothing.
DELETE FROM oauth_auth_requests WHERE id = $1 AND expires_at > NOW();

-- name: UpsertOAuthGrant :one
-- Consent creates the user's connected app or brings a revoked one back.
INSERT INTO oauth_grants (user_id, client_id, organization_id, brand_id, resource, scopes)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, client_id, resource, organization_id)
DO UPDATE SET scopes = EXCLUDED.scopes, revoked_at = NULL
RETURNING *;

-- name: CreateOAuthCode :exec
INSERT INTO oauth_codes (
    code_hash, grant_id, client_id, user_id, organization_id, brand_id,
    redirect_uri, code_challenge, resource, scopes, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: UseOAuthCode :one
-- Marking the code used is the claim; an expired or used code returns no row.
UPDATE oauth_codes
SET used_at = NOW(), family = sqlc.arg(family)
WHERE code_hash = sqlc.arg(code_hash) AND used_at IS NULL AND expires_at > NOW()
RETURNING *;

-- name: GetUsedOAuthCodeFamily :one
SELECT family FROM oauth_codes WHERE code_hash = $1 AND used_at IS NOT NULL;

-- name: CreateOAuthToken :exec
INSERT INTO oauth_tokens (
    token_hash, kind, family, grant_id, client_id, user_id, organization_id,
    brand_id, resource, scopes, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetActiveOAuthToken :one
-- A live token of a live grant and client, with the user's status.
SELECT t.*, u.status AS user_status
FROM oauth_tokens t
JOIN users u ON u.id = t.user_id
JOIN oauth_grants g ON g.id = t.grant_id
JOIN oauth_clients c ON c.client_id = t.client_id
WHERE t.token_hash = $1 AND t.kind = $2
  AND t.revoked_at IS NULL AND t.expires_at > NOW()
  AND g.revoked_at IS NULL AND c.revoked_at IS NULL;

-- name: GetOAuthTokenAnyState :one
SELECT * FROM oauth_tokens WHERE token_hash = $1 AND kind = $2;

-- name: RevokeOAuthToken :execrows
-- Revoking is the claim: of two concurrent refreshes only one wins.
UPDATE oauth_tokens SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeOAuthFamily :exec
UPDATE oauth_tokens SET revoked_at = NOW() WHERE family = $1 AND revoked_at IS NULL;

-- name: TouchOAuthToken :exec
-- Last use of the token and its grant (connected apps list).
WITH t AS (
    UPDATE oauth_tokens SET last_used_at = NOW() WHERE oauth_tokens.id = $1
    RETURNING grant_id
)
UPDATE oauth_grants SET last_used_at = NOW() WHERE oauth_grants.id = (SELECT grant_id FROM t);

-- name: CleanupOAuthAuthRequests :execrows
DELETE FROM oauth_auth_requests WHERE expires_at < NOW();

-- name: CleanupOAuthCodes :execrows
-- Used codes stay one day past expiry so a late replay still revokes.
DELETE FROM oauth_codes WHERE expires_at < NOW() - INTERVAL '1 day';

-- name: CleanupOAuthTokens :execrows
-- Rotated refresh tokens stay until they expire, for reuse detection.
DELETE FROM oauth_tokens WHERE expires_at < NOW() - INTERVAL '1 day';

-- name: CleanupOAuthClients :execrows
-- Registrations never connected within a day (abandoned DCR attempts).
DELETE FROM oauth_clients c
WHERE c.created_at < NOW() - INTERVAL '1 day'
  AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.client_id)
  AND NOT EXISTS (SELECT 1 FROM oauth_auth_requests r WHERE r.client_id = c.client_id);
