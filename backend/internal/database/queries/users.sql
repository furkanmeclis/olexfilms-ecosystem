-- name: CreateUser :one
INSERT INTO users (email, password_hash, name, surname, status, email_verified_at, phone_e164, phone_verified_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetUserByPhone :one
SELECT * FROM users
WHERE phone_e164 = $1 AND deleted_at IS NULL;

-- name: MarkUserPhoneVerified :exec
UPDATE users
SET phone_verified_at = COALESCE(phone_verified_at, NOW())
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByUUID :one
SELECT * FROM users
WHERE uuid = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserPasswordByID :exec
UPDATE users
SET password_hash = $2
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserProfileBasics :exec
UPDATE users
SET name = $2,
    surname = $3,
    status = $4,
    email_verified_at = COALESCE(email_verified_at, NOW())
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserPlatform :one
UPDATE users
SET name = COALESCE(sqlc.narg(name), name),
    surname = COALESCE(sqlc.narg(surname), surname),
    status = COALESCE(sqlc.narg(status), status)
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserProfileByUUID :one
UPDATE users
SET name = COALESCE(sqlc.narg(name), name),
    surname = COALESCE(sqlc.narg(surname), surname),
    locale = COALESCE(sqlc.narg(locale), locale)
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserLocale :exec
UPDATE users
SET locale = $2
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsersFiltered :many
SELECT DISTINCT u.*
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status))
  AND (sqlc.narg(role_slug)::text IS NULL OR r.slug = sqlc.narg(role_slug))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY u.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountUsers :one
SELECT COUNT(DISTINCT u.id)::bigint
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status))
  AND (sqlc.narg(role_slug)::text IS NULL OR r.slug = sqlc.narg(role_slug))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: CountUsersWithRole :one
SELECT COUNT(DISTINCT u.id)::bigint
FROM users u
INNER JOIN user_roles ur ON ur.user_id = u.id
INNER JOIN roles r ON r.id = ur.role_id
WHERE u.deleted_at IS NULL
  AND u.status <> 'disabled'
  AND r.slug = sqlc.arg(role_slug);

-- name: ListUsersForExport :many
SELECT DISTINCT u.*
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status))
  AND (sqlc.narg(role_slug)::text IS NULL OR r.slug = sqlc.narg(role_slug))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY u.created_at DESC;

-- name: ListUserUUIDsForBulk :many
SELECT DISTINCT u.uuid
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status))
  AND (sqlc.narg(role_slug)::text IS NULL OR r.slug = sqlc.narg(role_slug))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY u.created_at DESC;
