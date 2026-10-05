-- TEC-350 (F3-09a): admin-defined review questions, their translations, the
-- answers on a service review, and the review flags (migration 000091).
-- Questions are bounded by brand; answer reads are bounded by resolved
-- organization ids where the API layer owns scope resolution.

-- name: ListReviewQuestionsByBrand :many
-- active_only = true lists only the active questions (the portal form).
SELECT * FROM review_questions
WHERE brand_id = sqlc.arg(brand_id)::bigint
  AND (NOT sqlc.arg(active_only)::boolean OR is_active)
ORDER BY sort_order, id;

-- name: GetReviewQuestionByUUID :one
SELECT * FROM review_questions
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)::bigint;

-- name: GetReviewQuestionByID :one
SELECT * FROM review_questions
WHERE id = sqlc.arg(id)::bigint AND brand_id = sqlc.arg(brand_id)::bigint;

-- name: CreateReviewQuestion :one
INSERT INTO review_questions (
    brand_id, question_key, question_type, target, is_required, is_active, sort_order
) VALUES (
    sqlc.arg(brand_id)::bigint, sqlc.arg(question_key)::varchar,
    sqlc.arg(question_type)::varchar, sqlc.arg(target)::varchar,
    sqlc.arg(is_required)::boolean, sqlc.arg(is_active)::boolean,
    sqlc.arg(sort_order)::int
)
RETURNING *;

-- name: UpdateReviewQuestion :one
-- question_key, question_type and target are fixed once answers exist; the
-- use case decides whether to allow changing them, this query rewrites all.
UPDATE review_questions
SET question_key = sqlc.arg(question_key)::varchar,
    question_type = sqlc.arg(question_type)::varchar,
    target = sqlc.arg(target)::varchar,
    is_required = sqlc.arg(is_required)::boolean,
    sort_order = sqlc.arg(sort_order)::int,
    updated_at = NOW()
WHERE id = sqlc.arg(id)::bigint AND brand_id = sqlc.arg(brand_id)::bigint
RETURNING *;

-- name: SetReviewQuestionActive :one
UPDATE review_questions
SET is_active = sqlc.arg(is_active)::boolean,
    updated_at = NOW()
WHERE id = sqlc.arg(id)::bigint AND brand_id = sqlc.arg(brand_id)::bigint
RETURNING *;

-- name: ReviewQuestionHasAnswers :one
SELECT EXISTS (
    SELECT 1 FROM service_review_answers WHERE question_id = sqlc.arg(question_id)::bigint
)::boolean;

-- name: UpsertReviewQuestionLocale :one
INSERT INTO review_question_locales (question_id, locale, text)
VALUES (sqlc.arg(question_id)::bigint, sqlc.arg(locale)::varchar, sqlc.arg(text)::varchar)
ON CONFLICT (question_id, locale) DO UPDATE
SET text = EXCLUDED.text, updated_at = NOW()
RETURNING *;

-- name: DeleteReviewQuestionLocale :exec
DELETE FROM review_question_locales
WHERE question_id = sqlc.arg(question_id)::bigint AND locale = sqlc.arg(locale)::varchar;

-- name: ListReviewQuestionLocales :many
SELECT * FROM review_question_locales
WHERE question_id = sqlc.arg(question_id)::bigint
ORDER BY locale;

-- name: ListReviewQuestionLocalesByQuestions :many
-- Batch load for a question list (avoids N+1).
SELECT * FROM review_question_locales
WHERE question_id = ANY(sqlc.arg(question_ids)::bigint[])
ORDER BY question_id, locale;

-- name: CreateServiceReviewAnswer :one
-- The trigger enforces org/brand = review, question brand = review brand,
-- type/rating/text agreement and product target <=> product_id.
INSERT INTO service_review_answers (
    review_id, organization_id, brand_id, question_id, product_id, rating, text
) VALUES (
    sqlc.arg(review_id)::bigint, sqlc.arg(organization_id)::bigint,
    sqlc.arg(brand_id)::bigint, sqlc.arg(question_id)::bigint,
    sqlc.narg(product_id)::bigint, sqlc.narg(rating)::smallint,
    sqlc.narg(text)::varchar
)
RETURNING *;

-- name: ListServiceReviewAnswersByReview :many
SELECT * FROM service_review_answers
WHERE review_id = sqlc.arg(review_id)::bigint
ORDER BY question_id, product_id NULLS FIRST, id;

-- name: ListServiceReviewAnswersByReviews :many
-- Batch load for a review list (avoids N+1).
SELECT * FROM service_review_answers
WHERE review_id = ANY(sqlc.arg(review_ids)::bigint[])
ORDER BY review_id, question_id, product_id NULLS FIRST, id;

-- name: ListServiceReviewAnswersByQuestion :many
-- organization_ids bounds the read to the caller's scope; NULL means no
-- organization bound (scope all/brand, already bounded by brand_id).
SELECT * FROM service_review_answers
WHERE question_id = sqlc.arg(question_id)::bigint
  AND brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL
       OR organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountServiceReviewAnswersByQuestion :one
SELECT COUNT(*) FROM service_review_answers
WHERE question_id = sqlc.arg(question_id)::bigint
  AND brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL
       OR organization_id = ANY(sqlc.narg(organization_ids)::bigint[]));

-- name: SetServiceReviewFlags :one
UPDATE service_reviews
SET is_anonymous = sqlc.arg(is_anonymous)::boolean,
    source = sqlc.arg(source)::varchar
WHERE id = sqlc.arg(id)::bigint
RETURNING *;

-- name: MarkServiceReviewProcessed :one
-- Idempotent: an already processed review returns no row (pgx.ErrNoRows).
UPDATE service_reviews
SET processed_at = NOW()
WHERE id = sqlc.arg(id)::bigint AND processed_at IS NULL
RETURNING *;

-- name: ListUnprocessedServiceReviews :many
SELECT * FROM service_reviews
WHERE processed_at IS NULL
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit);

-- name: GetServiceReviewProcessingDetails :one
SELECT sr.id, sr.uuid, sr.organization_id, sr.brand_id, sr.service_id,
       sr.customer_user_id, sr.platform_rating, sr.product_rating, sr.comment,
       sr.created_at, sr.is_anonymous, sr.source, sr.processed_at,
       s.uuid AS service_uuid, s.service_no, s.plate,
       o.name AS organization_name,
       center.id AS center_organization_id,
       u.name AS customer_name, u.surname AS customer_surname,
       COALESCE(
           array_remove(array_agg(DISTINCT om.user_id) FILTER (
               WHERE om.user_id IS NOT NULL
                 AND (om.role = 'owner' OR r.slug = 'dealer_owner')
           ), NULL),
           ARRAY[]::bigint[]
       )::bigint[] AS dealer_owner_user_ids,
       LEAST(
           sr.platform_rating,
           sr.product_rating,
           COALESCE((
               SELECT MIN(a.rating)
               FROM service_review_answers a
               WHERE a.review_id = sr.id AND a.rating IS NOT NULL
           ), 5)
       )::smallint AS min_rating
FROM service_reviews sr
JOIN services s ON s.id = sr.service_id
JOIN organizations o ON o.id = sr.organization_id
JOIN organizations center ON center.brand_id = sr.brand_id AND center.type = 'center'
JOIN users u ON u.id = sr.customer_user_id
LEFT JOIN organization_members om ON om.organization_id = sr.organization_id
LEFT JOIN organization_member_roles mr ON mr.member_id = om.id
LEFT JOIN roles r ON r.id = mr.role_id
WHERE sr.id = sqlc.arg(review_id)::bigint
GROUP BY sr.id, s.uuid, s.service_no, s.plate, o.name, center.id, u.name, u.surname;
