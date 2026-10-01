-- name: ListModules :many
SELECT * FROM modules ORDER BY sort_order ASC, key ASC;

-- name: GetModule :one
SELECT * FROM modules WHERE key = $1;

-- name: UpsertModuleCatalog :exec
-- Catalog sync: level and sort order follow the Go catalog; admin-edited
-- default_enabled / is_paid survive (a core module is always on).
INSERT INTO modules (key, level, default_enabled, is_paid, sort_order)
VALUES (sqlc.arg(key), sqlc.arg(level), sqlc.arg(default_enabled), sqlc.arg(is_paid), sqlc.arg(sort_order))
ON CONFLICT (key) DO UPDATE SET
    level = EXCLUDED.level,
    sort_order = EXCLUDED.sort_order,
    default_enabled = CASE WHEN EXCLUDED.level = 'core' THEN true ELSE modules.default_enabled END;

-- name: UpdateModuleDefaults :one
UPDATE modules
SET default_enabled = COALESCE(sqlc.narg(default_enabled), default_enabled),
    is_paid = COALESCE(sqlc.narg(is_paid), is_paid)
WHERE key = sqlc.arg(key)
RETURNING *;

-- name: ListModuleFlagsForOrgs :many
-- System rows plus the org / dealer_standard rows of the given organizations.
SELECT f.id, f.scope, f.organization_id, f.module_key, f.enabled, f.source,
       f.set_by_user_id, f.service_id, f.note, f.created_at, f.updated_at,
       u.uuid AS set_by_uuid,
       COALESCE(NULLIF(TRIM(CONCAT(u.name, ' ', u.surname)), ''), '')::text AS set_by_name
FROM module_flags f
LEFT JOIN users u ON u.id = f.set_by_user_id
WHERE f.scope = 'system' OR f.organization_id = ANY (sqlc.arg(org_ids)::bigint[])
ORDER BY f.id;

-- name: UpsertSystemModuleFlag :one
INSERT INTO module_flags (scope, organization_id, module_key, enabled, source, set_by_user_id, note)
VALUES ('system', NULL, sqlc.arg(module_key), sqlc.arg(enabled), 'admin', sqlc.narg(set_by_user_id), sqlc.narg(note))
ON CONFLICT (module_key) WHERE scope = 'system' DO UPDATE SET
    enabled = EXCLUDED.enabled,
    source = EXCLUDED.source,
    set_by_user_id = EXCLUDED.set_by_user_id,
    note = EXCLUDED.note
RETURNING *;

-- name: UpsertOrgModuleFlag :one
INSERT INTO module_flags (scope, organization_id, module_key, enabled, source, set_by_user_id, note)
VALUES (sqlc.arg(scope), sqlc.arg(organization_id), sqlc.arg(module_key), sqlc.arg(enabled),
        sqlc.arg(source), sqlc.narg(set_by_user_id), sqlc.narg(note))
ON CONFLICT (scope, organization_id, module_key) WHERE organization_id IS NOT NULL DO UPDATE SET
    enabled = EXCLUDED.enabled,
    source = EXCLUDED.source,
    set_by_user_id = EXCLUDED.set_by_user_id,
    note = EXCLUDED.note
RETURNING *;

-- name: GetOrgModuleFlag :one
SELECT * FROM module_flags
WHERE scope = sqlc.arg(scope) AND organization_id = sqlc.arg(organization_id) AND module_key = sqlc.arg(module_key);

-- name: DeleteOrgModuleFlag :execrows
DELETE FROM module_flags
WHERE scope = sqlc.arg(scope) AND organization_id = sqlc.arg(organization_id) AND module_key = sqlc.arg(module_key);

-- name: DeleteSystemModuleFlag :execrows
DELETE FROM module_flags WHERE scope = 'system' AND module_key = $1;

-- name: ListOrganizationOwnerUserIDs :many
SELECT om.user_id
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = $1 AND om.role = 'owner'
ORDER BY om.user_id;

-- name: ListUserIDsByRoleSlug :many
SELECT ur.user_id
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
JOIN users u ON u.id = ur.user_id AND u.deleted_at IS NULL
WHERE r.slug = $1
ORDER BY ur.user_id;
