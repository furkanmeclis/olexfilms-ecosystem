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
ORDER BY sr.created_at DESC, sr.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountServiceReviewsInScope :one
SELECT COUNT(*)
FROM service_reviews sr
WHERE sr.brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL
       OR sr.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]));
