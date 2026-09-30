-- name: InsertStorageStar :one
INSERT INTO storage_stars (user_id, object_key)
VALUES ($1, $2)
ON CONFLICT (user_id, object_key) DO UPDATE SET object_key = EXCLUDED.object_key
RETURNING *;

-- name: DeleteStorageStar :exec
DELETE FROM storage_stars
WHERE user_id = $1 AND object_key = $2;

-- name: ListStorageStarsByUser :many
SELECT * FROM storage_stars
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListStorageStarsByKeys :many
SELECT * FROM storage_stars
WHERE user_id = $1 AND object_key = ANY(sqlc.arg(keys)::text[]);

-- name: InsertStorageTrash :one
INSERT INTO storage_trash (object_key, original_key, name, size_bytes, mime_type, deleted_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetStorageTrashByOriginalKey :one
SELECT * FROM storage_trash
WHERE original_key = $1
ORDER BY deleted_at DESC
LIMIT 1;

-- name: GetStorageTrashByUUID :one
SELECT * FROM storage_trash
WHERE uuid = $1;

-- name: ListStorageTrash :many
SELECT * FROM storage_trash
ORDER BY deleted_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStorageTrash :one
SELECT COUNT(*)::bigint FROM storage_trash;

-- name: DeleteStorageTrashByUUID :exec
DELETE FROM storage_trash WHERE uuid = $1;

-- name: InsertStorageLink :one
INSERT INTO storage_links (
    object_key, kind, slug, token_hash, expires_at,
    can_view, can_download, can_upload, created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetStorageLinkByUUID :one
SELECT * FROM storage_links WHERE uuid = $1;

-- name: GetStorageLinkBySlug :one
SELECT * FROM storage_links
WHERE slug = $1 AND revoked_at IS NULL;

-- name: GetStorageLinkByTokenHash :one
SELECT * FROM storage_links
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: ListStorageLinksByKey :many
SELECT * FROM storage_links
WHERE object_key = $1
ORDER BY created_at DESC;

-- name: ListActivePublicLinks :many
SELECT * FROM storage_links
WHERE kind = 'public'
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY created_at DESC;

-- name: RevokeStorageLink :one
UPDATE storage_links
SET revoked_at = NOW()
WHERE uuid = $1
RETURNING *;

-- name: ListActivePublicKeys :many
SELECT DISTINCT object_key FROM storage_links
WHERE kind = 'public'
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW())
  AND object_key = ANY(sqlc.arg(keys)::text[]);

-- name: InsertStorageShare :one
INSERT INTO storage_shares (object_key, user_id, role, created_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (object_key, user_id) DO UPDATE SET role = EXCLUDED.role
RETURNING *;

-- name: DeleteStorageShare :exec
DELETE FROM storage_shares WHERE uuid = $1;

-- name: ListStorageSharesByKey :many
SELECT s.*, u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname, u.email AS user_email
FROM storage_shares s
INNER JOIN users u ON u.id = s.user_id
WHERE s.object_key = $1
ORDER BY s.created_at ASC;

-- name: ListStorageSharesForUser :many
SELECT * FROM storage_shares
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListSharedKeys :many
SELECT DISTINCT object_key FROM storage_shares
WHERE object_key = ANY(sqlc.arg(keys)::text[]);

-- name: InsertStorageActivity :one
INSERT INTO storage_activity (object_key, actor_user_id, action, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListStorageActivity :many
SELECT a.*, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM storage_activity a
LEFT JOIN users u ON u.id = a.actor_user_id
WHERE a.object_key = $1
ORDER BY a.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStorageActivity :one
SELECT COUNT(*)::bigint FROM storage_activity
WHERE object_key = $1;
