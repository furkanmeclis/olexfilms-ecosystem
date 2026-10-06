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
-- Sort: docs/list-contract.md, keys from auth/model.RolesSortSpec (TEC-365).
SELECT *
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
  AND (sqlc.narg(is_system)::bool IS NULL OR is_system = sqlc.narg(is_system))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN name WHEN 'slug' THEN slug END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN name WHEN 'slug' THEN slug END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'is_system' THEN is_system END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'is_system' THEN is_system END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRoles :one
SELECT COUNT(*)::bigint
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
  AND (sqlc.narg(is_system)::bool IS NULL OR is_system = sqlc.narg(is_system));

-- name: ListRolesForExport :many
SELECT *
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
  AND (sqlc.narg(is_system)::bool IS NULL OR is_system = sqlc.narg(is_system))
ORDER BY is_system DESC, name ASC;

-- name: ListRoleUUIDsForBulk :many
SELECT uuid
FROM roles
WHERE (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
)
  AND (sqlc.narg(is_system)::bool IS NULL OR is_system = sqlc.narg(is_system))
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
    OR module ILIKE '%' || sqlc.narg(q) || '%'
)
ORDER BY sort_order, slug
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPermissions :one
SELECT COUNT(*)::bigint
FROM permissions
WHERE (
    sqlc.narg(q)::text IS NULL
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR module ILIKE '%' || sqlc.narg(q) || '%'
);

-- name: GetPermissionBySlug :one
SELECT * FROM permissions
WHERE slug = $1;

-- name: SetRolePermissions :exec
DELETE FROM role_permissions
WHERE role_id = $1;

-- name: InsertRolePermission :exec
INSERT INTO role_permissions (role_id, permission_id, scope)
VALUES ($1, $2, sqlc.arg(scope))
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- name: ListRolePermissionSlugsByRoleUUID :many
SELECT p.slug
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
INNER JOIN roles r ON r.id = rp.role_id
WHERE r.uuid = $1
ORDER BY p.slug;

-- name: ListRoleGrantsByRoleUUID :many
SELECT p.slug, rp.scope
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
INNER JOIN roles r ON r.id = rp.role_id
WHERE r.uuid = $1
ORDER BY p.sort_order, p.slug;

-- name: ListRoleGrantsByRoleID :many
SELECT p.slug, rp.scope
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.sort_order, p.slug;

-- name: ListGrantsByRoleSlugs :many
-- Grants of global roles (user_roles / JWT roles claim).
SELECT r.slug AS role_slug, p.slug AS permission_slug, rp.scope
FROM role_permissions rp
INNER JOIN roles r ON r.id = rp.role_id
INNER JOIN permissions p ON p.id = rp.permission_id
WHERE r.slug = ANY (sqlc.arg(role_slugs)::text[])
ORDER BY r.slug, p.slug;

-- name: ListMemberGrants :many
-- Grants of the user's roles in one organization (active org context).
SELECT r.slug AS role_slug, p.slug AS permission_slug, rp.scope
FROM organization_members om
INNER JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
INNER JOIN organization_member_roles mr ON mr.member_id = om.id
INNER JOIN roles r ON r.id = mr.role_id
INNER JOIN role_permissions rp ON rp.role_id = r.id
INNER JOIN permissions p ON p.id = rp.permission_id
WHERE om.user_id = sqlc.arg(user_id) AND o.uuid = sqlc.arg(organization_uuid)
ORDER BY r.slug, p.slug;

-- name: ListMemberRoleSlugs :many
SELECT r.slug
FROM organization_members om
INNER JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
INNER JOIN organization_member_roles mr ON mr.member_id = om.id
INNER JOIN roles r ON r.id = mr.role_id
WHERE om.user_id = sqlc.arg(user_id) AND o.uuid = sqlc.arg(organization_uuid)
ORDER BY r.slug;

-- name: ListAllPermissions :many
SELECT * FROM permissions ORDER BY sort_order, slug;

-- name: UpsertPermission :exec
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
VALUES (
    sqlc.arg(name), sqlc.arg(slug), sqlc.arg(module), sqlc.arg(scopes)::text[],
    sqlc.arg(is_sensitive), sqlc.arg(super_admin_only), sqlc.narg(description), sqlc.arg(sort_order)
)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    module = EXCLUDED.module,
    scopes = EXCLUDED.scopes,
    is_sensitive = EXCLUDED.is_sensitive,
    super_admin_only = EXCLUDED.super_admin_only,
    description = EXCLUDED.description,
    sort_order = EXCLUDED.sort_order;

-- name: DeletePermissionBySlug :exec
DELETE FROM permissions WHERE slug = $1;

-- name: ListAllRoles :many
SELECT * FROM roles ORDER BY slug;

-- name: UpsertSystemRole :one
INSERT INTO roles (name, slug, description, is_system, org_type)
VALUES (sqlc.arg(name), sqlc.arg(slug), sqlc.narg(description), true, sqlc.narg(org_type))
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    is_system = true,
    org_type = EXCLUDED.org_type
RETURNING *;

-- name: DeleteRolePermission :exec
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.role_id = sqlc.arg(role_id)
  AND rp.permission_id = p.id
  AND p.slug = sqlc.arg(permission_slug);
