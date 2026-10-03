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
-- Author view: announcements written by the given organizations (the
-- caller's resolved write scope), newest first.
SELECT * FROM announcements
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAnnouncementsByOrganizations :one
SELECT COUNT(*) FROM announcements
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar);

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
