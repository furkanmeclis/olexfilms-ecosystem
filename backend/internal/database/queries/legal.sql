-- Portal legal texts and consents (TEC-90).

-- name: GetLatestLegalText :one
SELECT * FROM legal_texts
WHERE kind = $1 AND locale = $2
ORDER BY version DESC
LIMIT 1;

-- name: ListLatestLegalTexts :many
SELECT DISTINCT ON (locale) *
FROM legal_texts
WHERE kind = $1
ORDER BY locale, version DESC;

-- name: ListLegalTextVersions :many
SELECT * FROM legal_texts
WHERE kind = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: InsertLegalText :one
INSERT INTO legal_texts (kind, locale, version, body, created_by)
SELECT sqlc.arg(kind)::text, sqlc.arg(locale)::text, COALESCE(MAX(version), 0) + 1,
       sqlc.arg(body)::text, sqlc.narg(created_by)::bigint
FROM legal_texts
WHERE kind = sqlc.arg(kind)::text AND locale = sqlc.arg(locale)::text
RETURNING *;

-- name: GetConsentForText :one
SELECT * FROM consents
WHERE user_id = $1 AND legal_text_id = $2;

-- name: InsertConsent :one
INSERT INTO consents (user_id, legal_text_id, kind, locale, text_version, accepted, ip, user_agent)
VALUES (sqlc.arg(user_id), sqlc.arg(legal_text_id), sqlc.arg(kind), sqlc.arg(locale),
        sqlc.arg(text_version), sqlc.arg(accepted), sqlc.narg(ip), sqlc.narg(user_agent))
ON CONFLICT (user_id, legal_text_id) DO NOTHING
RETURNING *;

-- name: GetLatestConsent :one
SELECT * FROM consents
WHERE user_id = $1 AND kind = $2
ORDER BY decided_at DESC, id DESC
LIMIT 1;
