-- TEC-244 (F2-03h): the portal service review form (one per service).

-- name: GetServiceReviewByService :one
SELECT * FROM service_reviews
WHERE service_id = sqlc.arg(service_id)::bigint;

-- name: CreateServiceReview :one
-- ON CONFLICT DO NOTHING: a second review of the same service returns no
-- row (pgx.ErrNoRows), which the use case answers with 409.
INSERT INTO service_reviews (
    organization_id, brand_id, service_id, customer_user_id,
    platform_rating, product_rating, comment, is_anonymous, source
) VALUES (
    sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint,
    sqlc.arg(service_id)::bigint, sqlc.arg(customer_user_id)::bigint,
    sqlc.arg(platform_rating)::smallint, sqlc.arg(product_rating)::smallint,
    sqlc.narg(comment)::varchar, sqlc.arg(is_anonymous)::boolean,
    sqlc.arg(source)::varchar
)
ON CONFLICT (service_id) DO NOTHING
RETURNING *;

-- name: ListServiceReviewProducts :many
SELECT DISTINCT p.id, p.uuid, p.sku, p.name
FROM service_items si
JOIN products p ON p.id = si.product_id AND p.brand_id = si.brand_id
WHERE si.service_id = sqlc.arg(service_id)::bigint
ORDER BY p.name, p.id;

-- name: ListServiceReviewsInScope :many
SELECT sr.id, sr.uuid, sr.organization_id, sr.brand_id, sr.service_id,
       sr.customer_user_id, sr.platform_rating, sr.product_rating, sr.comment,
       sr.created_at, sr.is_anonymous, sr.source, sr.processed_at,
       s.uuid AS service_uuid, s.service_no, s.plate,
       u.uuid AS customer_uuid, u.name AS customer_name, u.surname AS customer_surname,
       u.phone_e164 AS customer_phone
FROM service_reviews sr
JOIN services s ON s.id = sr.service_id
JOIN users u ON u.id = sr.customer_user_id
WHERE sr.brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL
       OR sr.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
  AND (sqlc.narg(dealer_uuid)::uuid IS NULL
       OR EXISTS (
           SELECT 1 FROM organizations ro
           WHERE ro.id = sr.organization_id AND ro.uuid = sqlc.narg(dealer_uuid)::uuid
       ))
  AND (sqlc.narg(product_uuid)::uuid IS NULL
       OR EXISTS (
           SELECT 1
           FROM service_items si
           JOIN products p ON p.id = si.product_id AND p.brand_id = si.brand_id
           WHERE si.service_id = sr.service_id AND p.uuid = sqlc.narg(product_uuid)::uuid
       ))
  AND (sqlc.narg(min_rating)::smallint IS NULL
       OR sr.platform_rating >= sqlc.narg(min_rating)::smallint
       OR sr.product_rating >= sqlc.narg(min_rating)::smallint)
  AND (sqlc.narg(max_rating)::smallint IS NULL
       OR sr.platform_rating <= sqlc.narg(max_rating)::smallint
       OR sr.product_rating <= sqlc.narg(max_rating)::smallint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR sr.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR sr.created_at < sqlc.narg(created_to)::timestamptz)
ORDER BY sr.created_at DESC, sr.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountServiceReviewsInScope :one
SELECT COUNT(*)
FROM service_reviews sr
WHERE sr.brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL
       OR sr.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
  AND (sqlc.narg(dealer_uuid)::uuid IS NULL
       OR EXISTS (
           SELECT 1 FROM organizations ro
           WHERE ro.id = sr.organization_id AND ro.uuid = sqlc.narg(dealer_uuid)::uuid
       ))
  AND (sqlc.narg(product_uuid)::uuid IS NULL
       OR EXISTS (
           SELECT 1
           FROM service_items si
           JOIN products p ON p.id = si.product_id AND p.brand_id = si.brand_id
           WHERE si.service_id = sr.service_id AND p.uuid = sqlc.narg(product_uuid)::uuid
       ))
  AND (sqlc.narg(min_rating)::smallint IS NULL
       OR sr.platform_rating >= sqlc.narg(min_rating)::smallint
       OR sr.product_rating >= sqlc.narg(min_rating)::smallint)
  AND (sqlc.narg(max_rating)::smallint IS NULL
       OR sr.platform_rating <= sqlc.narg(max_rating)::smallint
       OR sr.product_rating <= sqlc.narg(max_rating)::smallint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR sr.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR sr.created_at < sqlc.narg(created_to)::timestamptz);

-- name: ReviewDealerStats :many
WITH scoped AS (
    SELECT sr.*
    FROM service_reviews sr
    WHERE sr.brand_id = sqlc.arg(brand_id)::bigint
      AND (sqlc.narg(organization_ids)::bigint[] IS NULL
           OR sr.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
      AND (sqlc.narg(created_from)::timestamptz IS NULL OR sr.created_at >= sqlc.narg(created_from)::timestamptz)
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR sr.created_at < sqlc.narg(created_to)::timestamptz)
),
ratings AS (
    SELECT organization_id, platform_rating::numeric AS rating FROM scoped
    UNION ALL
    SELECT a.organization_id, a.rating::numeric
    FROM service_review_answers a
    JOIN review_questions q ON q.id = a.question_id
    JOIN scoped sr ON sr.id = a.review_id
    WHERE a.rating IS NOT NULL AND q.target IN ('platform', 'dealer')
)
SELECT o.uuid AS dealer_uuid, o.name AS dealer_name,
       COUNT(DISTINCT sr.id)::bigint AS review_count,
       ROUND(AVG(r.rating), 2)::numeric AS average_rating
FROM scoped sr
JOIN organizations o ON o.id = sr.organization_id
LEFT JOIN ratings r ON r.organization_id = sr.organization_id
GROUP BY o.uuid, o.name
ORDER BY average_rating DESC NULLS LAST, o.name;

-- name: ReviewProductStats :many
WITH scoped AS (
    SELECT sr.*
    FROM service_reviews sr
    WHERE sr.brand_id = sqlc.arg(brand_id)::bigint
      AND (sqlc.narg(organization_ids)::bigint[] IS NULL
           OR sr.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
      AND (sqlc.narg(created_from)::timestamptz IS NULL OR sr.created_at >= sqlc.narg(created_from)::timestamptz)
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR sr.created_at < sqlc.narg(created_to)::timestamptz)
),
ratings AS (
    SELECT si.product_id, sr.id AS review_id, sr.product_rating::numeric AS rating
    FROM scoped sr
    JOIN service_items si ON si.service_id = sr.service_id
    UNION ALL
    SELECT a.product_id, a.review_id, a.rating::numeric
    FROM service_review_answers a
    JOIN review_questions q ON q.id = a.question_id
    JOIN scoped sr ON sr.id = a.review_id
    WHERE a.rating IS NOT NULL AND q.target = 'product' AND a.product_id IS NOT NULL
)
SELECT p.uuid AS product_uuid, p.sku, p.name AS product_name,
       COUNT(DISTINCT r.review_id)::bigint AS review_count,
       ROUND(AVG(r.rating), 2)::numeric AS average_rating
FROM ratings r
JOIN products p ON p.id = r.product_id
GROUP BY p.uuid, p.sku, p.name
ORDER BY average_rating DESC NULLS LAST, p.name;
