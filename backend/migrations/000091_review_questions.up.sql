-- TEC-350 (F3-09a): admin-defined review questions on top of the TEC-244
-- service review form.
--
--   * review_questions: questions the center defines per brand. question_type
--     rating_1_5 | text, target platform | dealer | product. The two fixed
--     ratings on service_reviews (platform_rating, product_rating) stay as
--     they are; admin questions are extra questions.
--   * review_question_locales: the question text per language (13 locales).
--   * service_review_answers: one answer per (review, question, product).
--     product_id is set only for product questions (the product of a service
--     item). A rating_1_5 answer carries a 1-5 rating and no text; a text
--     answer carries text and no rating (trigger, check_violation).
--   * service_reviews gains is_anonymous, source (portal | whatsapp_link) and
--     processed_at (set by the F3-09c processing job).
--
-- The reviews module flag (000031: add-on, off by default) is not changed
-- here; see TEC-120.

-- 1. service_reviews: new columns ------------------------------------------
ALTER TABLE service_reviews
    ADD COLUMN is_anonymous BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN source       VARCHAR(16) NOT NULL DEFAULT 'portal',
    ADD COLUMN processed_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_service_reviews_source CHECK (source IN ('portal', 'whatsapp_link'));

CREATE INDEX idx_service_reviews_unprocessed ON service_reviews (created_at)
    WHERE processed_at IS NULL;

-- 2. review_questions --------------------------------------------------------
CREATE TABLE review_questions (
    id             BIGSERIAL    PRIMARY KEY,
    uuid           UUID         NOT NULL DEFAULT gen_random_uuid(),
    brand_id       BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    question_key   VARCHAR(64)  NOT NULL,
    question_type  VARCHAR(16)  NOT NULL,
    target         VARCHAR(16)  NOT NULL,
    is_required    BOOLEAN      NOT NULL DEFAULT false,
    is_active      BOOLEAN      NOT NULL DEFAULT true,
    sort_order     INTEGER      NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_review_questions_uuid UNIQUE (uuid),
    CONSTRAINT uq_review_questions_key UNIQUE (brand_id, question_key),
    CONSTRAINT chk_review_questions_key CHECK (question_key ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT chk_review_questions_type CHECK (question_type IN ('rating_1_5', 'text')),
    CONSTRAINT chk_review_questions_target CHECK (target IN ('platform', 'dealer', 'product'))
);

CREATE INDEX idx_review_questions_brand_active ON review_questions (brand_id, sort_order, id)
    WHERE is_active;

-- 3. review_question_locales ------------------------------------------------
CREATE TABLE review_question_locales (
    question_id  BIGINT       NOT NULL REFERENCES review_questions (id) ON DELETE CASCADE,
    locale       VARCHAR(8)   NOT NULL,
    text         VARCHAR(500) NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (question_id, locale),
    CONSTRAINT chk_review_question_locales_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
    ),
    CONSTRAINT chk_review_question_locales_text CHECK (btrim(text) <> '')
);

-- 4. service_review_answers -------------------------------------------------
CREATE TABLE service_review_answers (
    id               BIGSERIAL     PRIMARY KEY,
    review_id        BIGINT        NOT NULL REFERENCES service_reviews (id) ON DELETE CASCADE,
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    question_id      BIGINT        NOT NULL REFERENCES review_questions (id) ON DELETE RESTRICT,
    product_id       BIGINT        NULL REFERENCES products (id) ON DELETE RESTRICT,
    rating           SMALLINT      NULL,
    text             VARCHAR(2000) NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_review_answers UNIQUE NULLS NOT DISTINCT (review_id, question_id, product_id),
    CONSTRAINT chk_service_review_answers_rating CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
    CONSTRAINT chk_service_review_answers_text CHECK (text IS NULL OR btrim(text) <> '')
);

CREATE INDEX idx_service_review_answers_question ON service_review_answers (question_id, created_at DESC);
CREATE INDEX idx_service_review_answers_org ON service_review_answers (organization_id, created_at DESC);
CREATE INDEX idx_service_review_answers_product ON service_review_answers (product_id)
    WHERE product_id IS NOT NULL;

-- An answer follows its review (org, brand) and its question (brand, type,
-- target): rating_1_5 answers have a rating and no text, text answers have
-- text and no rating; product questions need a product, the others none.
CREATE FUNCTION service_review_answers_check() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    rv service_reviews%ROWTYPE;
    q  review_questions%ROWTYPE;
BEGIN
    SELECT * INTO rv FROM service_reviews WHERE id = NEW.review_id;
    SELECT * INTO q FROM review_questions WHERE id = NEW.question_id;
    IF NEW.organization_id <> rv.organization_id OR NEW.brand_id <> rv.brand_id THEN
        RAISE EXCEPTION 'service_review_answers: organization/brand must match the review'
            USING ERRCODE = 'check_violation';
    END IF;
    IF q.brand_id <> rv.brand_id THEN
        RAISE EXCEPTION 'service_review_answers: question belongs to another brand'
            USING ERRCODE = 'check_violation';
    END IF;
    IF q.question_type = 'rating_1_5' AND (NEW.rating IS NULL OR NEW.text IS NOT NULL) THEN
        RAISE EXCEPTION 'service_review_answers: rating question needs a rating and no text'
            USING ERRCODE = 'check_violation';
    END IF;
    IF q.question_type = 'text' AND (NEW.rating IS NOT NULL OR NEW.text IS NULL) THEN
        RAISE EXCEPTION 'service_review_answers: text question needs text and no rating'
            USING ERRCODE = 'check_violation';
    END IF;
    IF (q.target = 'product') <> (NEW.product_id IS NOT NULL) THEN
        RAISE EXCEPTION 'service_review_answers: product_id is required for product questions only'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_review_answers_check
    BEFORE INSERT OR UPDATE ON service_review_answers
    FOR EACH ROW
    EXECUTE FUNCTION service_review_answers_check();

-- 5. Permissions -----------------------------------------------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage review questions', 'reviews.questions.manage', 'reviews',
       ARRAY['brand', 'all']::text[], false, false,
       'Define the extra review questions and their translations (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read service reviews', 'reviews.read', 'reviews',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read service reviews and their answers (dealer: own, distributor: subtree, center: brand).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'reviews.questions.manage', 'all'),
    ('super_admin', 'reviews.read', 'all'),
    ('center_staff', 'reviews.questions.manage', 'brand'),
    ('center_staff', 'reviews.read', 'brand'),
    ('center_social', 'reviews.read', 'brand'),
    ('distributor_owner', 'reviews.read', 'subtree'),
    ('distributor_staff', 'reviews.read', 'subtree'),
    ('dealer_owner', 'reviews.read', 'managed'),
    ('dealer_staff', 'reviews.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
