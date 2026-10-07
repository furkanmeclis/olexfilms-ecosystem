-- TEC-329 (F3-05a): document library folders, items and append-only file
-- versions (migration 000086).

-- name: CreateLibraryFolder :one
INSERT INTO library_folders (organization_id, brand_id, parent_id, name, sort_order, created_by_user_id)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(parent_id), sqlc.arg(name),
    sqlc.arg(sort_order), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetLibraryFolderByUUID :one
SELECT * FROM library_folders WHERE uuid = $1 AND deleted_at IS NULL;

-- name: ListLibraryFolders :many
SELECT * FROM library_folders
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY parent_id NULLS FIRST, sort_order, lower(name), id;

-- name: ListLibraryFoldersByBrand :many
-- Reader view: the brand's folder tree (folders are center-managed; item
-- visibility is still filtered per item).
SELECT * FROM library_folders
WHERE brand_id = $1 AND deleted_at IS NULL
ORDER BY parent_id NULLS FIRST, sort_order, lower(name), id;

-- name: UpdateLibraryFolder :one
UPDATE library_folders
SET name       = sqlc.arg(name),
    parent_id  = sqlc.narg(parent_id),
    sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteLibraryFolder :execrows
-- Only an empty folder (no live subfolder or item) is removed.
UPDATE library_folders f
SET deleted_at = NOW()
WHERE f.id = sqlc.arg(id) AND f.organization_id = sqlc.arg(organization_id) AND f.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM library_folders c WHERE c.parent_id = f.id AND c.deleted_at IS NULL)
  AND NOT EXISTS (SELECT 1 FROM library_items i WHERE i.folder_id = f.id AND i.deleted_at IS NULL);

-- name: CreateLibraryItem :one
INSERT INTO library_items (
    organization_id, brand_id, folder_id, name, description, tags, access_level, role_slug, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(folder_id), sqlc.arg(name),
    sqlc.narg(description), sqlc.arg(tags)::text[], sqlc.arg(access_level), sqlc.narg(role_slug),
    sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetLibraryItemByUUID :one
SELECT * FROM library_items WHERE uuid = $1 AND deleted_at IS NULL;

-- name: GetLibraryItemByID :one
SELECT * FROM library_items WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateLibraryItem :one
UPDATE library_items
SET folder_id    = sqlc.narg(folder_id),
    name         = sqlc.arg(name),
    description  = sqlc.narg(description),
    tags         = sqlc.arg(tags)::text[],
    access_level = sqlc.arg(access_level),
    role_slug    = sqlc.narg(role_slug)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteLibraryItem :execrows
UPDATE library_items
SET deleted_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL;

-- name: ListLibraryItems :many
-- Reader view in a brand. access_levels are the levels the viewer
-- organization may see (center: all four; distributor: all_network and
-- distributors; dealer: all_network and dealers); an item with a role_slug
-- is shown only to viewers holding that role. folder_id NULL lists every
-- folder; tags matches items carrying any of the given tags; access_filter
-- narrows the visible levels. Sort: docs/list-contract.md, keys from
-- usecase.ItemsSortSpec.
SELECT * FROM library_items
WHERE brand_id = sqlc.arg(brand_id)::bigint
  AND deleted_at IS NULL
  AND access_level = ANY(sqlc.arg(access_levels)::text[])
  AND (role_slug IS NULL OR role_slug = ANY(sqlc.arg(viewer_role_slugs)::text[]))
  AND (sqlc.narg(folder_id)::bigint IS NULL OR folder_id = sqlc.narg(folder_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(tags)::text[]), 0) = 0 OR tags && sqlc.narg(tags)::text[])
  AND (
      COALESCE(cardinality(sqlc.narg(access_filter)::text[]), 0) = 0
      OR access_level = ANY(sqlc.narg(access_filter)::text[])
  )
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR updated_at >= sqlc.narg(updated_from)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR updated_at < sqlc.narg(updated_before)::timestamptz)
  AND (
      sqlc.narg(q)::text IS NULL
      OR name ILIKE ('%' || sqlc.narg(q)::text || '%')
      OR COALESCE(description, '') ILIKE ('%' || sqlc.narg(q)::text || '%')
      OR EXISTS (
          SELECT 1 FROM unnest(tags) AS tag_value
          WHERE tag_value ILIKE ('%' || sqlc.narg(q)::text || '%')
      )
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(name) WHEN 'access_level' THEN access_level END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(name) WHEN 'access_level' THEN access_level END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountLibraryItems :one
-- Same filter block as ListLibraryItems.
SELECT COUNT(*)::bigint FROM library_items
WHERE brand_id = sqlc.arg(brand_id)::bigint
  AND deleted_at IS NULL
  AND access_level = ANY(sqlc.arg(access_levels)::text[])
  AND (role_slug IS NULL OR role_slug = ANY(sqlc.arg(viewer_role_slugs)::text[]))
  AND (sqlc.narg(folder_id)::bigint IS NULL OR folder_id = sqlc.narg(folder_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(tags)::text[]), 0) = 0 OR tags && sqlc.narg(tags)::text[])
  AND (
      COALESCE(cardinality(sqlc.narg(access_filter)::text[]), 0) = 0
      OR access_level = ANY(sqlc.narg(access_filter)::text[])
  )
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR updated_at >= sqlc.narg(updated_from)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR updated_at < sqlc.narg(updated_before)::timestamptz)
  AND (
      sqlc.narg(q)::text IS NULL
      OR name ILIKE ('%' || sqlc.narg(q)::text || '%')
      OR COALESCE(description, '') ILIKE ('%' || sqlc.narg(q)::text || '%')
      OR EXISTS (
          SELECT 1 FROM unnest(tags) AS tag_value
          WHERE tag_value ILIKE ('%' || sqlc.narg(q)::text || '%')
      )
  );

-- name: NextLibraryItemVersionNo :one
SELECT (COALESCE(MAX(version_no), 0) + 1)::int AS next_version
FROM library_item_versions
WHERE item_id = sqlc.arg(item_id) AND locale = sqlc.arg(locale);

-- name: CreateLibraryItemVersion :one
INSERT INTO library_item_versions (
    item_id, locale, version_no, storage_key, mime, size_bytes, sha256, uploaded_by_user_id
) VALUES (
    sqlc.arg(item_id), sqlc.arg(locale), sqlc.arg(version_no), sqlc.arg(storage_key), sqlc.arg(mime),
    sqlc.arg(size_bytes), sqlc.arg(sha256), sqlc.narg(uploaded_by_user_id)
)
RETURNING *;

-- name: ListLibraryItemVersions :many
SELECT * FROM library_item_versions
WHERE item_id = $1
ORDER BY locale, version_no DESC;

-- name: ListLatestLibraryItemVersions :many
-- The newest version of each language of an item.
SELECT DISTINCT ON (locale) *
FROM library_item_versions
WHERE item_id = $1
ORDER BY locale, version_no DESC;

-- name: GetLibraryItemVersionByUUID :one
SELECT * FROM library_item_versions WHERE uuid = $1;
