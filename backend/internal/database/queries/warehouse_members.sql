-- TEC-769: user <-> warehouse membership and organization warehouse settings.

-- name: WarehouseMembershipCheck :one
-- has_any: the user has at least one membership in the organization (the
-- restriction is on); is_member: one of them is the given warehouse.
SELECT
    EXISTS (SELECT 1 FROM warehouse_members a
            WHERE a.organization_id = sqlc.arg(organization_id) AND a.user_id = sqlc.arg(user_id))::bool AS has_any,
    EXISTS (SELECT 1 FROM warehouse_members b
            WHERE b.organization_id = sqlc.arg(organization_id) AND b.user_id = sqlc.arg(user_id)
              AND b.warehouse_id = sqlc.arg(warehouse_id))::bool AS is_member;

-- name: ListWarehouseMembers :many
-- List contract: sort keys name, email, created_at (id tiebreak); q on
-- name, surname, email.
SELECT m.*, u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname, u.email AS user_email
FROM warehouse_members m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = sqlc.arg(organization_id) AND m.warehouse_id = sqlc.arg(warehouse_id)
  AND (sqlc.narg(q)::text IS NULL
       OR u.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR u.surname ILIKE '%' || sqlc.narg(q)::text || '%'
       OR u.email ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(u.name || ' ' || u.surname) WHEN 'email' THEN lower(u.email) END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(u.name || ' ' || u.surname) WHEN 'email' THEN lower(u.email) END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN m.id END DESC,
  m.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWarehouseMembers :one
SELECT count(*) FROM warehouse_members m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = sqlc.arg(organization_id) AND m.warehouse_id = sqlc.arg(warehouse_id)
  AND (sqlc.narg(q)::text IS NULL
       OR u.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR u.surname ILIKE '%' || sqlc.narg(q)::text || '%'
       OR u.email ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: GetWarehouseMember :one
SELECT m.*, u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname, u.email AS user_email
FROM warehouse_members m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = sqlc.arg(organization_id) AND m.warehouse_id = sqlc.arg(warehouse_id)
  AND m.user_id = sqlc.arg(user_id);

-- name: InsertWarehouseMember :one
INSERT INTO warehouse_members (organization_id, warehouse_id, user_id, is_default, created_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(warehouse_id), sqlc.arg(user_id), sqlc.arg(is_default), sqlc.narg(created_by_user_id))
ON CONFLICT (warehouse_id, user_id) DO UPDATE SET is_default = warehouse_members.is_default OR EXCLUDED.is_default
RETURNING *;

-- name: ClearWarehouseMemberDefault :exec
UPDATE warehouse_members SET is_default = false
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id) AND is_default;

-- name: SetWarehouseMemberDefault :execrows
UPDATE warehouse_members SET is_default = true
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id)
  AND warehouse_id = sqlc.arg(warehouse_id);

-- name: DeleteWarehouseMember :execrows
DELETE FROM warehouse_members
WHERE organization_id = sqlc.arg(organization_id) AND warehouse_id = sqlc.arg(warehouse_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListUserWarehouseMemberships :many
SELECT m.warehouse_id, m.is_default FROM warehouse_members m
WHERE m.organization_id = sqlc.arg(organization_id) AND m.user_id = sqlc.arg(user_id);

-- name: GetOrganizationMemberUser :one
-- An active user that is a member of the organization (member picker).
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id) AND u.uuid = sqlc.arg(user_uuid);

-- name: GetWarehouseSettings :one
SELECT * FROM warehouse_settings WHERE organization_id = sqlc.arg(organization_id);

-- name: UpsertWarehouseSettings :one
INSERT INTO warehouse_settings (organization_id, eod_cutoff_minutes)
VALUES (sqlc.arg(organization_id), sqlc.narg(eod_cutoff_minutes))
ON CONFLICT (organization_id) DO UPDATE SET eod_cutoff_minutes = EXCLUDED.eod_cutoff_minutes
RETURNING *;
