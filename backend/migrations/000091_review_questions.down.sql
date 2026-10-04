DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('reviews.questions.manage', 'reviews.read')
);
DELETE FROM permissions WHERE slug IN ('reviews.questions.manage', 'reviews.read');

DROP TRIGGER IF EXISTS trg_service_review_answers_check ON service_review_answers;
DROP FUNCTION IF EXISTS service_review_answers_check();
DROP TABLE IF EXISTS service_review_answers;
DROP TABLE IF EXISTS review_question_locales;
DROP TABLE IF EXISTS review_questions;

DROP INDEX IF EXISTS idx_service_reviews_unprocessed;
ALTER TABLE service_reviews
    DROP CONSTRAINT IF EXISTS chk_service_reviews_source,
    DROP COLUMN IF EXISTS processed_at,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS is_anonymous;
