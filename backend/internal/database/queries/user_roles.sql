-- name: ListUserRoleSlugs :many
SELECT r.slug
FROM roles r
INNER JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.slug;

-- name: ListUserRolesByUserID :many
SELECT r.*
FROM roles r
INNER JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.slug;

-- name: ListUserRolesByUserUUID :many
SELECT r.*
FROM roles r
INNER JOIN user_roles ur ON ur.role_id = r.id
INNER JOIN users u ON u.id = ur.user_id
WHERE u.uuid = $1 AND u.deleted_at IS NULL
ORDER BY r.slug;

-- name: ListRolesForUserIDs :many
SELECT ur.user_id, r.uuid, r.name, r.slug, r.description, r.is_system
FROM user_roles ur
INNER JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ANY (sqlc.arg(user_ids)::bigint[])
ORDER BY ur.user_id, r.slug;

-- name: ReplaceUserRoles :exec
DELETE FROM user_roles
WHERE user_id = $1;

-- name: InsertUserRole :exec
INSERT INTO user_roles (user_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: AssignUserRoleBySlug :exec
INSERT INTO user_roles (user_id, role_id)
SELECT $1, r.id
FROM roles r
WHERE r.slug = $2
ON CONFLICT DO NOTHING;

-- name: RemoveUserRoleBySlug :exec
DELETE FROM user_roles ur
USING roles r
WHERE ur.user_id = $1
  AND ur.role_id = r.id
  AND r.slug = $2;

-- name: UserHasRoleSlug :one
SELECT EXISTS (
    SELECT 1
    FROM user_roles ur
    INNER JOIN roles r ON r.id = ur.role_id
    WHERE ur.user_id = $1 AND r.slug = $2
)::boolean AS has_role;
