-- TEC-329 (F3-05a): announcements, their translations, audiences and read
-- receipts (migration 000086).

-- name: CreateAnnouncement :one
INSERT INTO announcements (
    organization_id, brand_id, default_locale, title, body, body_format,
    status, pinned, notify, publish_at, expires_at, author_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(default_locale), sqlc.arg(title),
    sqlc.arg(body), sqlc.arg(body_format), sqlc.arg(status), sqlc.arg(pinned), sqlc.arg(notify),
    sqlc.narg(publish_at), sqlc.narg(expires_at), sqlc.narg(author_user_id)
)
RETURNING *;

-- name: GetAnnouncementByUUID :one
SELECT * FROM announcements WHERE uuid = $1;

-- name: GetAnnouncementByID :one
SELECT * FROM announcements WHERE id = $1;

-- name: UpdateAnnouncement :one
-- Content and flags of an announcement written by the organization.
UPDATE announcements
SET default_locale = sqlc.arg(default_locale),
    title          = sqlc.arg(title),
    body           = sqlc.arg(body),
    body_format    = sqlc.arg(body_format),
    pinned         = sqlc.arg(pinned),
    notify         = sqlc.arg(notify),
    publish_at     = sqlc.narg(publish_at),
    expires_at     = sqlc.narg(expires_at)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: SetAnnouncementStatus :one
-- publish_at is stamped with NOW() when a row is published without one.
UPDATE announcements
SET status     = sqlc.arg(status)::varchar,
    publish_at = CASE
        WHEN sqlc.arg(status)::varchar = 'published' AND publish_at IS NULL THEN NOW()
        ELSE publish_at
    END
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: DeleteAnnouncement :execrows
DELETE FROM announcements
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'draft';

-- name: ListAnnouncementsByOrganizations :many
-- Author view (TEC-367): announcements written by the given organizations
-- (the caller's write scope) in every status. Sort keys from
-- usecase.AdminSortSpec (docs/list-contract.md); default -created_at.
SELECT * FROM announcements
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (sqlc.narg(pinned)::bool IS NULL OR pinned = sqlc.narg(pinned)::bool)
  AND (sqlc.narg(publish_from)::timestamptz IS NULL OR publish_at >= sqlc.narg(publish_from))
  AND (sqlc.narg(publish_before)::timestamptz IS NULL OR publish_at < sqlc.narg(publish_before))
  AND (sqlc.narg(q)::text IS NULL OR title ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'title' THEN title::text WHEN 'status' THEN status::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'title' THEN title::text WHEN 'status' THEN status::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'publish_at' THEN publish_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'publish_at' THEN publish_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAnnouncementsByOrganizations :one
SELECT COUNT(*) FROM announcements
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (sqlc.narg(pinned)::bool IS NULL OR pinned = sqlc.narg(pinned)::bool)
  AND (sqlc.narg(publish_from)::timestamptz IS NULL OR publish_at >= sqlc.narg(publish_from))
  AND (sqlc.narg(publish_before)::timestamptz IS NULL OR publish_at < sqlc.narg(publish_before))
  AND (sqlc.narg(q)::text IS NULL OR title ILIKE '%' || sqlc.narg(q) || '%');

-- name: UpsertAnnouncementLocale :one
INSERT INTO announcement_locales (announcement_id, locale, title, body)
VALUES (sqlc.arg(announcement_id), sqlc.arg(locale), sqlc.arg(title), sqlc.arg(body))
ON CONFLICT (announcement_id, locale)
DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body
RETURNING *;

-- name: ListAnnouncementLocales :many
SELECT * FROM announcement_locales
WHERE announcement_id = $1
ORDER BY locale;

-- name: DeleteAnnouncementLocale :execrows
DELETE FROM announcement_locales
WHERE announcement_id = sqlc.arg(announcement_id) AND locale = sqlc.arg(locale);

-- name: AddAnnouncementAudience :one
INSERT INTO announcement_audiences (announcement_id, target_type, role_slug, target_organization_id)
VALUES (sqlc.arg(announcement_id), sqlc.arg(target_type), sqlc.narg(role_slug), sqlc.narg(target_organization_id))
RETURNING *;

-- name: ListAnnouncementAudiences :many
SELECT * FROM announcement_audiences
WHERE announcement_id = $1
ORDER BY id;

-- name: DeleteAnnouncementAudiences :exec
DELETE FROM announcement_audiences WHERE announcement_id = $1;

-- name: ListVisibleAnnouncements :many
-- Reader feed: published, inside its window, in the viewer's brand and
-- matching at least one audience row. viewer_org_lineage is the viewer
-- organization followed by its ancestors (subtree targets match any of
-- them); viewer_role_slugs are the viewer's role slugs in that
-- organization. locale picks the translation, falling back to the default
-- text.
SELECT
    a.id, a.uuid, a.organization_id, a.default_locale, a.body_format, a.pinned, a.publish_at, a.expires_at,
    COALESCE(l.locale, a.default_locale)::varchar AS locale,
    COALESCE(l.title, a.title)::varchar AS title,
    COALESCE(l.body, a.body)::text AS body,
    r.read_at
FROM announcements a
LEFT JOIN announcement_locales l
       ON l.announcement_id = a.id AND l.locale = sqlc.arg(locale)::varchar
LEFT JOIN announcement_reads r
       ON r.announcement_id = a.id AND r.user_id = sqlc.arg(user_id)::bigint
WHERE a.brand_id = sqlc.arg(brand_id)::bigint
  AND a.status = 'published'
  AND a.publish_at <= sqlc.arg(now)::timestamptz
  AND (a.expires_at IS NULL OR a.expires_at > sqlc.arg(now)::timestamptz)
  AND EXISTS (
      SELECT 1 FROM announcement_audiences au
      WHERE au.announcement_id = a.id
        AND (au.role_slug IS NULL OR au.role_slug = ANY(sqlc.arg(viewer_role_slugs)::text[]))
        AND (
            au.target_type IN ('all_network', 'role')
            OR (au.target_type = 'distributors' AND sqlc.arg(viewer_org_type)::varchar = 'distributor')
            OR (au.target_type = 'dealers' AND sqlc.arg(viewer_org_type)::varchar = 'dealer')
            OR (au.target_type = 'subtree' AND au.target_organization_id = ANY(sqlc.arg(viewer_org_lineage)::bigint[]))
        )
  )
  AND (NOT sqlc.arg(unread_only)::boolean OR r.read_at IS NULL)
ORDER BY a.pinned DESC, a.publish_at DESC, a.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountVisibleAnnouncements :one
SELECT COUNT(*)
FROM announcements a
LEFT JOIN announcement_reads r
       ON r.announcement_id = a.id AND r.user_id = sqlc.arg(user_id)::bigint
WHERE a.brand_id = sqlc.arg(brand_id)::bigint
  AND a.status = 'published'
  AND a.publish_at <= sqlc.arg(now)::timestamptz
  AND (a.expires_at IS NULL OR a.expires_at > sqlc.arg(now)::timestamptz)
  AND EXISTS (
      SELECT 1 FROM announcement_audiences au
      WHERE au.announcement_id = a.id
        AND (au.role_slug IS NULL OR au.role_slug = ANY(sqlc.arg(viewer_role_slugs)::text[]))
        AND (
            au.target_type IN ('all_network', 'role')
            OR (au.target_type = 'distributors' AND sqlc.arg(viewer_org_type)::varchar = 'distributor')
            OR (au.target_type = 'dealers' AND sqlc.arg(viewer_org_type)::varchar = 'dealer')
            OR (au.target_type = 'subtree' AND au.target_organization_id = ANY(sqlc.arg(viewer_org_lineage)::bigint[]))
        )
  )
  AND (NOT sqlc.arg(unread_only)::boolean OR r.read_at IS NULL);

-- name: MarkAnnouncementRead :one
-- Idempotent: a second read keeps the first read_at.
INSERT INTO announcement_reads (announcement_id, user_id)
VALUES (sqlc.arg(announcement_id), sqlc.arg(user_id))
ON CONFLICT (announcement_id, user_id) DO UPDATE SET read_at = announcement_reads.read_at
RETURNING *;

-- name: CountAnnouncementReads :one
SELECT COUNT(*) FROM announcement_reads WHERE announcement_id = $1;

-- name: ListAnnouncementReads :many
SELECT * FROM announcement_reads
WHERE announcement_id = sqlc.arg(announcement_id)
ORDER BY read_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListAnnouncementTargetUserIDs :many
WITH RECURSIVE subtree(root_id, id) AS (
    SELECT au.target_organization_id, au.target_organization_id
    FROM announcement_audiences au
    WHERE au.announcement_id = sqlc.arg(announcement_id)
      AND au.target_type = 'subtree'
      AND au.target_organization_id IS NOT NULL
    UNION ALL
    SELECT s.root_id, o.id
    FROM subtree s
    JOIN organizations o ON o.parent_id = s.id AND o.deleted_at IS NULL
)
SELECT DISTINCT om.user_id
FROM announcements a
JOIN organization_members om ON TRUE
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE a.id = sqlc.arg(announcement_id)
  AND a.brand_id = o.brand_id
  AND EXISTS (
      SELECT 1
      FROM announcement_audiences au
      WHERE au.announcement_id = a.id
        AND (
            au.role_slug IS NULL
            OR EXISTS (
                SELECT 1
                FROM organization_member_roles mr
                JOIN roles r ON r.id = mr.role_id
                WHERE mr.member_id = om.id AND r.slug = au.role_slug
            )
        )
        AND (
            au.target_type IN ('all_network', 'role')
            OR (au.target_type = 'distributors' AND o.type = 'distributor')
            OR (au.target_type = 'dealers' AND o.type = 'dealer')
            OR (au.target_type = 'subtree' AND EXISTS (
                SELECT 1 FROM subtree s WHERE s.root_id = au.target_organization_id AND s.id = o.id
            ))
        )
  )
ORDER BY om.user_id;

-- name: ListAnnouncementReadReport :many
SELECT u.uuid AS user_uuid, u.email, u.name, u.surname, r.read_at
FROM announcement_reads r
JOIN users u ON u.id = r.user_id AND u.deleted_at IS NULL
WHERE r.announcement_id = sqlc.arg(announcement_id)
ORDER BY r.read_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: OrganizationLineage :many
WITH RECURSIVE lineage(id, parent_id, depth) AS (
    SELECT id, parent_id, 0
    FROM organizations
    WHERE id = sqlc.arg(id)::bigint AND deleted_at IS NULL
    UNION ALL
    SELECT p.id, p.parent_id, lineage.depth + 1
    FROM lineage
    JOIN organizations p ON p.id = lineage.parent_id AND p.deleted_at IS NULL
    WHERE lineage.depth < 16
)
SELECT id FROM lineage ORDER BY depth ASC;
