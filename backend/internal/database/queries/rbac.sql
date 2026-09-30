-- name: GetRoleBySlug :one
SELECT * FROM roles
WHERE slug = $1;

-- name: GetRoleByUUID :one
SELECT * FROM roles
WHERE uuid = $1;

-- name: GetRoleByID :one
SELECT * FROM roles
WHERE id = $1;

-- name: ListRolesFiltered :many
SELECT *
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
ORDER BY is_system DESC, name ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRoles :one
SELECT COUNT(*)::bigint
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
);

-- name: ListRolesForExport :many
SELECT *
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
ORDER BY is_system DESC, name ASC;

-- name: ListRoleUUIDsForBulk :many
SELECT uuid
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
ORDER BY is_system DESC, name ASC;

-- name: CreateRole :one
INSERT INTO roles (name, slug, description, is_system)
VALUES ($1, $2, $3, false)
RETURNING *;

-- name: UpdateRole :one
UPDATE roles
SET name = COALESCE(sqlc.narg(name), name),
    description = COALESCE(sqlc.narg(description), description)
WHERE uuid = sqlc.arg(uuid)
  AND is_system = false
RETURNING *;

-- name: DeleteRole :exec
DELETE FROM roles
WHERE uuid = $1
  AND is_system = false;

-- name: ListPermissionSlugsByRoleID :many
SELECT p.slug
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.slug;

-- name: ListPermissionSlugsByRoleSlug :many
SELECT p.slug
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
INNER JOIN roles r ON r.id = rp.role_id
WHERE r.slug = $1
ORDER BY p.slug;

-- name: ListAllPermissionSlugs :many
SELECT slug FROM permissions ORDER BY slug;

-- name: ListPermissionsFiltered :many
SELECT *
FROM permissions
WHERE (
    sqlc.narg(q)::text IS NULL
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
    OR name ILIKE '%' || sqlc.narg(q) || '%'
)
ORDER BY slug
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPermissions :one
SELECT COUNT(*)::bigint
FROM permissions
WHERE (
    sqlc.narg(q)::text IS NULL
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
    OR name ILIKE '%' || sqlc.narg(q) || '%'
);

-- name: GetPermissionBySlug :one
SELECT * FROM permissions
WHERE slug = $1;

-- name: SetRolePermissions :exec
DELETE FROM role_permissions
WHERE role_id = $1;

-- name: InsertRolePermission :exec
INSERT INTO role_permissions (role_id, permission_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListRolePermissionSlugsByRoleUUID :many
SELECT p.slug
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
INNER JOIN roles r ON r.id = rp.role_id
WHERE r.uuid = $1
ORDER BY p.slug;
