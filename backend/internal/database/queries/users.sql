-- name: CreateUser :one
INSERT INTO users (email, password_hash, name, surname, status, email_verified_at, phone_e164, phone_verified_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetUserByPhone :one
SELECT * FROM users
WHERE phone_e164 = $1 AND deleted_at IS NULL;

-- name: MarkUserPhoneVerified :exec
-- A verified phone also claims a migrated "unverified" customer (K26,
-- TEC-255): the legacy marker and the unresolved legacy phone are cleared.
UPDATE users
SET phone_verified_at = COALESCE(phone_verified_at, NOW()),
    legacy_unverified = FALSE,
    legacy_phone_raw  = NULL
WHERE id = $1 AND deleted_at IS NULL AND phone_e164 IS NOT NULL;

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
    locale = CASE WHEN sqlc.arg(set_locale)::bool THEN sqlc.narg(locale) ELSE locale END,
    timezone = CASE WHEN sqlc.arg(set_timezone)::bool THEN sqlc.narg(timezone) ELSE timezone END
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: GetLocaleSources :one
-- Stored locale/timezone preferences for i18n.Resolve: the user, the active
-- organization (when given) and the center of its brand, or of the request
-- brand when there is no active organization.
SELECT
    COALESCE(u.locale, '')::text   AS user_locale,
    COALESCE(u.timezone, '')::text AS user_timezone,
    COALESCE(o.locale, '')::text   AS org_locale,
    COALESCE(o.timezone, '')::text AS org_timezone,
    COALESCE(c.locale, '')::text   AS center_locale,
    COALESCE(c.timezone, '')::text AS center_timezone
FROM users u
LEFT JOIN organizations o
    ON o.uuid = sqlc.narg(organization_uuid)::uuid AND o.deleted_at IS NULL
LEFT JOIN organizations c
    ON c.type = 'center'
   AND c.deleted_at IS NULL
   AND c.brand_id = COALESCE(o.brand_id, sqlc.narg(brand_id)::bigint)
WHERE u.id = sqlc.arg(user_id) AND u.deleted_at IS NULL;

-- name: UpdateUserLocale :exec
UPDATE users
SET locale = $2
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsersFiltered :many
-- Platform users list. Sort follows docs/list-contract.md: sort_key is the
-- trusted key from apiquery.UsersSortSpec, one CASE pair per column type,
-- id as the unique tiebreak in the same direction.
SELECT u.*
FROM users u
WHERE u.deleted_at IS NULL
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR u.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(role_slugs)::text[]), 0) = 0
    OR EXISTS (
      SELECT 1 FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id
      WHERE ur.user_id = u.id AND r.slug = ANY (sqlc.narg(role_slugs)::text[])
    )
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'email' THEN u.email WHEN 'name' THEN u.name
      WHEN 'surname' THEN u.surname WHEN 'status' THEN u.status
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'email' THEN u.email WHEN 'name' THEN u.name
      WHEN 'surname' THEN u.surname WHEN 'status' THEN u.status
    END
  END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'created_at' THEN u.created_at WHEN 'updated_at' THEN u.updated_at
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'created_at' THEN u.created_at WHEN 'updated_at' THEN u.updated_at
    END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN u.id END DESC,
  u.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountUsers :one
SELECT COUNT(*)::bigint
FROM users u
WHERE u.deleted_at IS NULL
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR u.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(role_slugs)::text[]), 0) = 0
    OR EXISTS (
      SELECT 1 FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id
      WHERE ur.user_id = u.id AND r.slug = ANY (sqlc.narg(role_slugs)::text[])
    )
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before))
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
SELECT u.*
FROM users u
WHERE u.deleted_at IS NULL
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR u.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(role_slugs)::text[]), 0) = 0
    OR EXISTS (
      SELECT 1 FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id
      WHERE ur.user_id = u.id AND r.slug = ANY (sqlc.narg(role_slugs)::text[])
    )
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY u.created_at DESC, u.id DESC;

-- name: ListUserUUIDsForBulk :many
SELECT u.uuid
FROM users u
WHERE u.deleted_at IS NULL
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR u.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(role_slugs)::text[]), 0) = 0
    OR EXISTS (
      SELECT 1 FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id
      WHERE ur.user_id = u.id AND r.slug = ANY (sqlc.narg(role_slugs)::text[])
    )
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY u.created_at DESC, u.id DESC;
