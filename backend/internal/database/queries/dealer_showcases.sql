-- TEC-466 (F5-01a): dealer showcase. The usecase (F5-01b) checks the
-- module flag, the organization scope and the transition graph
-- (dealershowcase/model); these queries only guard the row state.

-- Showcase --------------------------------------------------------------------

-- name: GetDealerShowcaseByOrg :one
SELECT * FROM dealer_showcases
WHERE organization_id = sqlc.arg(organization_id);

-- name: GetDealerShowcaseByUUID :one
SELECT * FROM dealer_showcases
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockDealerShowcase :one
-- Serializes photo inserts and status moves of one showcase (the caller
-- runs in a transaction).
SELECT * FROM dealer_showcases
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: UpsertDealerShowcase :one
-- Creates the organization's showcase or replaces its draft content. The
-- status, the published snapshot and the Google rating are left alone: a
-- draft edit never touches the live page.
INSERT INTO dealer_showcases (organization_id, brand_id, content, working_hours, social_links, seo_keywords,
                              google_place_id, created_by_user_id, updated_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(content), sqlc.arg(working_hours),
        sqlc.arg(social_links), sqlc.arg(seo_keywords)::text[], sqlc.narg(google_place_id),
        sqlc.narg(actor_user_id), sqlc.narg(actor_user_id))
ON CONFLICT (organization_id) DO UPDATE
SET content            = EXCLUDED.content,
    working_hours      = EXCLUDED.working_hours,
    social_links       = EXCLUDED.social_links,
    seo_keywords       = EXCLUDED.seo_keywords,
    google_place_id    = EXCLUDED.google_place_id,
    updated_by_user_id = EXCLUDED.updated_by_user_id
RETURNING *;

-- name: SubmitDealerShowcase :one
-- CAS from_status → pending_review (approval on). No row when the status
-- moved meanwhile.
UPDATE dealer_showcases
SET status             = 'pending_review',
    submitted_at       = NOW(),
    review_note        = NULL,
    updated_by_user_id = sqlc.narg(actor_user_id)
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status)
RETURNING *;

-- name: PublishDealerShowcase :one
-- CAS from_status → published and writes the snapshot the public endpoint
-- reads. reviewer_user_id is set when the center approved a pending
-- showcase, NULL when the owner published directly (approval off).
UPDATE dealer_showcases
SET status            = 'published',
    published_content = sqlc.arg(published_content),
    published_at      = NOW(),
    reviewed_by       = sqlc.narg(reviewer_user_id),
    reviewed_at       = CASE WHEN sqlc.narg(reviewer_user_id)::bigint IS NULL THEN NULL ELSE NOW() END,
    review_note       = NULL,
    updated_by_user_id = COALESCE(sqlc.narg(reviewer_user_id), sqlc.narg(actor_user_id))
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status)
RETURNING *;

-- name: RejectDealerShowcase :one
-- CAS pending_review → rejected with the reviewer's note. The previous
-- published snapshot (if any) stays live.
UPDATE dealer_showcases
SET status             = 'rejected',
    reviewed_by        = sqlc.arg(reviewer_user_id),
    reviewed_at        = NOW(),
    review_note        = sqlc.arg(review_note),
    updated_by_user_id = sqlc.arg(reviewer_user_id)
WHERE id = sqlc.arg(id) AND status = 'pending_review'
RETURNING *;

-- name: SetDealerShowcaseGoogleRating :one
-- Writes the Google rating (Places worker or manual entry, F5-01d); NULL
-- rating and count clear it.
UPDATE dealer_showcases
SET google_rating            = sqlc.narg(google_rating),
    google_review_count      = sqlc.narg(google_review_count),
    google_rating_source     = sqlc.narg(google_rating_source),
    google_rating_updated_at = CASE WHEN sqlc.narg(google_rating)::numeric IS NULL THEN NULL ELSE NOW() END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListDealerShowcaseReviews :many
-- Center review queue (docs/list-contract.md): sort updated_at | name |
-- status (flow rank) with an id tiebreak; statuses multi-valued (empty = all);
-- q matches the organization name or city; updated_from / updated_before.
-- organization_ids narrows to a scope (empty = whole brand).
SELECT s.*,
       o.uuid AS organization_uuid,
       o.slug AS organization_slug,
       o.name AS organization_name,
       o.type AS organization_type,
       o.city AS organization_city
FROM dealer_showcases s
JOIN organizations o ON o.id = s.organization_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND (COALESCE(cardinality(sqlc.narg(organization_ids)::bigint[]), 0) = 0
       OR s.organization_id = ANY (sqlc.narg(organization_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.city ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR s.updated_at >= sqlc.narg(updated_from)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR s.updated_at < sqlc.narg(updated_before)::timestamptz)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN o.name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN o.name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE s.status WHEN 'draft' THEN 1 WHEN 'pending_review' THEN 2 WHEN 'published' THEN 3 ELSE 4 END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE s.status WHEN 'draft' THEN 1 WHEN 'pending_review' THEN 2 WHEN 'published' THEN 3 ELSE 4 END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'updated_at' THEN s.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'updated_at' THEN s.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN s.id END DESC,
  s.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDealerShowcaseReviews :one
SELECT COUNT(*)
FROM dealer_showcases s
JOIN organizations o ON o.id = s.organization_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND (COALESCE(cardinality(sqlc.narg(organization_ids)::bigint[]), 0) = 0
       OR s.organization_id = ANY (sqlc.narg(organization_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.city ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR s.updated_at >= sqlc.narg(updated_from)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR s.updated_at < sqlc.narg(updated_before)::timestamptz);

-- name: GetPublishedDealerShowcase :one
-- Public read of /bayi/{code}: the approved snapshot of an active, serving
-- dealer or distributor of the brand (same organization filters as
-- GetPublicDealerBySlug). A showcase that was published once keeps serving
-- its snapshot while a newer version waits for review or was rejected.
-- The module flag is checked by the usecase. The Google rating is live.
SELECT s.published_content,
       s.id AS showcase_id,
       s.published_at,
       s.google_place_id,
       s.google_rating,
       s.google_review_count,
       s.google_rating_source,
       s.google_rating_updated_at,
       o.id AS organization_id,
       o.locale AS organization_locale
FROM dealer_showcases s
JOIN organizations o ON o.id = s.organization_id
WHERE o.slug = sqlc.arg(slug)
  AND s.brand_id = sqlc.arg(brand_id)
  AND s.published_content IS NOT NULL
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type IN ('dealer', 'distributor')
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
  AND (o.contract_valid_until IS NULL OR o.contract_valid_until >= CURRENT_DATE);

-- name: GetShowcaseLeadTargetBySlug :one
-- TEC-468: public lead form is accepted only when the dealer showcase add-on
-- is enabled and the showcase has a published snapshot.
SELECT o.id AS organization_id,
       o.brand_id,
       o.slug,
       o.name,
       o.locale,
       o.country_id,
       c.iso2 AS country_iso2,
       s.id AS showcase_id,
       s.published_at
FROM organizations o
JOIN dealer_showcases s ON s.organization_id = o.id AND s.brand_id = o.brand_id
LEFT JOIN countries c ON c.id = o.country_id
WHERE o.slug = sqlc.arg(slug)
  AND o.brand_id = sqlc.arg(brand_id)
  AND s.published_content IS NOT NULL
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type IN ('dealer', 'distributor')
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
  AND (o.contract_valid_until IS NULL OR o.contract_valid_until >= CURRENT_DATE);

-- Services --------------------------------------------------------------------

-- name: ListDealerShowcaseServices :many
SELECT * FROM dealer_showcase_services
WHERE showcase_id = sqlc.arg(showcase_id)
ORDER BY sort_order, id;

-- name: InsertDealerShowcaseService :one
-- A new service goes to the end of the list.
INSERT INTO dealer_showcase_services (showcase_id, organization_id, brand_id, kind, category_id, title,
                                      description, visible, sort_order)
SELECT s.id, s.organization_id, s.brand_id, sqlc.arg(kind), sqlc.narg(category_id), sqlc.arg(title),
       sqlc.arg(description), sqlc.arg(visible),
       COALESCE((SELECT MAX(x.sort_order) FROM dealer_showcase_services x WHERE x.showcase_id = s.id), 0) + 10
FROM dealer_showcases s
WHERE s.id = sqlc.arg(showcase_id)
RETURNING *;

-- name: UpdateDealerShowcaseService :one
UPDATE dealer_showcase_services
SET kind        = sqlc.arg(kind),
    category_id = sqlc.narg(category_id),
    title       = sqlc.arg(title),
    description = sqlc.arg(description),
    visible     = sqlc.arg(visible)
WHERE uuid = sqlc.arg(uuid) AND showcase_id = sqlc.arg(showcase_id)
RETURNING *;

-- name: DeleteDealerShowcaseService :execrows
DELETE FROM dealer_showcase_services
WHERE uuid = sqlc.arg(uuid) AND showcase_id = sqlc.arg(showcase_id);

-- name: ReorderDealerShowcaseServices :execrows
-- Sets sort_order 10, 20, … in the given order; uuids of other showcases
-- are ignored (the caller compares the row count with the list length).
UPDATE dealer_showcase_services x
SET sort_order = o.ord * 10
FROM unnest(sqlc.arg(uuids)::uuid[]) WITH ORDINALITY AS o (uuid, ord)
WHERE x.uuid = o.uuid AND x.showcase_id = sqlc.arg(showcase_id);

-- Photos ----------------------------------------------------------------------

-- name: ListDealerShowcasePhotos :many
SELECT * FROM dealer_showcase_photos
WHERE showcase_id = sqlc.arg(showcase_id)
ORDER BY sort_order, id;

-- name: CountDealerShowcasePhotos :one
-- The usecase compares it with showcase.max_photos under LockDealerShowcase.
SELECT COUNT(*) FROM dealer_showcase_photos
WHERE showcase_id = sqlc.arg(showcase_id);

-- name: InsertDealerShowcasePhoto :one
-- A new photo goes to the end of the gallery.
INSERT INTO dealer_showcase_photos (showcase_id, organization_id, brand_id, storage_key, mime, size_bytes, sha256,
                                    caption, created_by_user_id, sort_order)
SELECT s.id, s.organization_id, s.brand_id, sqlc.arg(storage_key), sqlc.arg(mime), sqlc.arg(size_bytes),
       sqlc.arg(sha256), sqlc.arg(caption), sqlc.narg(created_by_user_id),
       COALESCE((SELECT MAX(x.sort_order) FROM dealer_showcase_photos x WHERE x.showcase_id = s.id), 0) + 10
FROM dealer_showcases s
WHERE s.id = sqlc.arg(showcase_id)
RETURNING *;

-- name: UpdateDealerShowcasePhotoCaption :one
UPDATE dealer_showcase_photos
SET caption = sqlc.arg(caption)
WHERE uuid = sqlc.arg(uuid) AND showcase_id = sqlc.arg(showcase_id)
RETURNING *;

-- name: DeleteDealerShowcasePhoto :one
-- Returns the storage key so the caller removes the object after commit.
DELETE FROM dealer_showcase_photos
WHERE uuid = sqlc.arg(uuid) AND showcase_id = sqlc.arg(showcase_id)
RETURNING storage_key;

-- name: ReorderDealerShowcasePhotos :execrows
UPDATE dealer_showcase_photos x
SET sort_order = o.ord * 10
FROM unnest(sqlc.arg(uuids)::uuid[]) WITH ORDINALITY AS o (uuid, ord)
WHERE x.uuid = o.uuid AND x.showcase_id = sqlc.arg(showcase_id);

-- TEC-467 (F5-01b) -------------------------------------------------------------

-- name: EnsureDealerShowcase :one
-- Opens an empty draft showcase for the organization when it has none
-- (a service or photo added before the first content save) and returns the
-- row either way.
WITH ins AS (
    INSERT INTO dealer_showcases (organization_id, brand_id, created_by_user_id, updated_by_user_id)
    VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(actor_user_id), sqlc.narg(actor_user_id))
    ON CONFLICT (organization_id) DO NOTHING
    RETURNING *
)
SELECT * FROM ins
UNION ALL
SELECT * FROM dealer_showcases WHERE organization_id = sqlc.arg(organization_id) AND NOT EXISTS (SELECT 1 FROM ins);

-- name: GetDealerShowcasePhoto :one
SELECT * FROM dealer_showcase_photos
WHERE uuid = sqlc.arg(uuid) AND showcase_id = sqlc.arg(showcase_id);

-- name: ListPublishedDealerShowcaseBadges :many
-- Nearby dealers list: which of the given organizations of the brand serve
-- a published showcase, with the live Google rating and its source and the
-- rating frozen in the snapshot (TEC-469: a manual rating under approval
-- shows the published one). The module flag is checked by the caller.
SELECT o.id AS organization_id,
       o.uuid AS organization_uuid,
       s.google_rating,
       s.google_rating_source,
       (s.published_content -> 'google_rating')::jsonb AS published_google_rating
FROM dealer_showcases s
JOIN organizations o ON o.id = s.organization_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND s.published_content IS NOT NULL
  AND o.uuid = ANY (sqlc.arg(organization_uuids)::uuid[]);

-- name: ListPublishedDealerShowcaseDates :many
-- Sitemap: the publish time of every published showcase of the brand (the
-- caller keeps the organizations whose module is on).
SELECT s.organization_id,
       o.slug,
       s.published_at
FROM dealer_showcases s
JOIN organizations o ON o.id = s.organization_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND s.published_content IS NOT NULL
  AND o.deleted_at IS NULL;

-- TEC-469 (F5-01d) -------------------------------------------------------------

-- name: ListDealerShowcasesForPlacesRefresh :many
-- Places worker: showcases with a Google place id whose rating did not come
-- from Places since fresh_before (a second run the same day finds nothing).
-- The module flag and the per-organization backoff are checked by the
-- caller.
SELECT s.id,
       s.uuid,
       s.organization_id,
       s.google_place_id,
       o.uuid AS organization_uuid
FROM dealer_showcases s
JOIN organizations o ON o.id = s.organization_id
WHERE s.google_place_id IS NOT NULL
  AND o.deleted_at IS NULL
  AND (s.google_rating_source IS DISTINCT FROM 'places'
       OR s.google_rating_updated_at IS NULL
       OR s.google_rating_updated_at < sqlc.arg(fresh_before)::timestamptz)
ORDER BY s.id;

-- name: SetDealerShowcasePlacesRating :execrows
-- Writes a Places answer. CAS on the place id: no row when the owner
-- changed the place id meanwhile.
UPDATE dealer_showcases
SET google_rating            = sqlc.arg(google_rating),
    google_review_count      = sqlc.arg(google_review_count),
    google_rating_source     = 'places',
    google_rating_updated_at = NOW()
WHERE id = sqlc.arg(id) AND google_place_id = sqlc.arg(google_place_id);
