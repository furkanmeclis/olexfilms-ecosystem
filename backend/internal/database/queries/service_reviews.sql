-- TEC-244 (F2-03h): the portal service review form (one per service).

-- name: GetServiceReviewByService :one
SELECT * FROM service_reviews
WHERE service_id = sqlc.arg(service_id)::bigint;

-- name: CreateServiceReview :one
-- ON CONFLICT DO NOTHING: a second review of the same service returns no
-- row (pgx.ErrNoRows), which the use case answers with 409.
INSERT INTO service_reviews (
    organization_id, brand_id, service_id, customer_user_id,
    platform_rating, product_rating, comment
) VALUES (
    sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint,
    sqlc.arg(service_id)::bigint, sqlc.arg(customer_user_id)::bigint,
    sqlc.arg(platform_rating)::smallint, sqlc.arg(product_rating)::smallint,
    sqlc.narg(comment)::varchar
)
ON CONFLICT (service_id) DO NOTHING
RETURNING *;
