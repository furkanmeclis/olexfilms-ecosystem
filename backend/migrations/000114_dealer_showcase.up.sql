-- TEC-466 (F5-01a): dealer showcase schema, permissions and sqlc. The API
-- (panel editor, photo upload, center review, public endpoint) lands in
-- F5-01b; this migration fixes the data model and database guards.
-- showcase.approval_required / showcase.max_photos live in the sysconfig
-- catalog (no row needed).

-- 1. Validators ---------------------------------------------------------------

-- Locale keyed text: an object whose keys are the 13 UI locales and whose
-- values are strings (titles, descriptions, captions).
CREATE FUNCTION dealer_showcase_locale_texts_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
              OR jsonb_typeof(e.value) <> 'string'
       )
$$;

-- Showcase content: {"tr": {"headline": "...", "about": "..."}, "en": {...}}.
-- Each locale holds an object of string fields; an empty or missing locale
-- falls back to the organization locale (application layer).
CREATE FUNCTION dealer_showcase_content_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
              OR jsonb_typeof(e.value) <> 'object'
              OR EXISTS (
                  SELECT 1 FROM jsonb_each(e.value) f
                  WHERE f.key NOT IN ('headline', 'about') OR jsonb_typeof(f.value) <> 'string'
              )
       )
$$;

-- Social links: known networks only, each an https URL.
CREATE FUNCTION dealer_showcase_social_links_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('instagram', 'facebook', 'youtube', 'tiktok', 'website')
              OR jsonb_typeof(e.value) <> 'string'
              OR (e.value #>> '{}') !~ '^https://[^\s/?#]+[^\s]*$'
              OR length(e.value #>> '{}') > 500
       )
$$;

-- SEO keywords: at most 30 trimmed, non-empty entries of up to 100 chars.
CREATE FUNCTION dealer_showcase_keywords_valid(k TEXT[]) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT COALESCE(cardinality(k), 0) <= 30
       AND NOT EXISTS (
           SELECT 1 FROM unnest(k) w
           WHERE w IS NULL OR btrim(w) = '' OR w <> btrim(w) OR char_length(w) > 100
       )
$$;

-- 2. dealer_showcases: one row per dealer or distributor ----------------------
-- status follows the latest submission: draft → pending_review →
-- published | rejected (or draft → published while approval is off).
-- published_content is the approved snapshot the public endpoint reads; it
-- survives later edits, re-submissions and rejections, so a draft never
-- breaks the live page.
CREATE TABLE dealer_showcases (
    id                       BIGSERIAL    PRIMARY KEY,
    uuid                     UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id          BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                 BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    status                   VARCHAR(16)  NOT NULL DEFAULT 'draft',
    content                  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    -- Same shape as appointment_settings.working_hours (weekday → [{start,
    -- end}]) so "copy appointment hours" is a plain copy; the application
    -- validates it with appointments.ValidateWorkingHours.
    working_hours            JSONB        NOT NULL DEFAULT '{}'::jsonb,
    social_links             JSONB        NOT NULL DEFAULT '{}'::jsonb,
    seo_keywords             TEXT[]       NOT NULL DEFAULT '{}'::text[],
    google_place_id          VARCHAR(255) NULL,
    google_rating            NUMERIC(2,1) NULL,
    google_review_count      INTEGER      NULL,
    google_rating_source     VARCHAR(8)   NULL,
    google_rating_updated_at TIMESTAMPTZ  NULL,
    published_content        JSONB        NULL,
    published_at             TIMESTAMPTZ  NULL,
    submitted_at             TIMESTAMPTZ  NULL,
    reviewed_by              BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at              TIMESTAMPTZ  NULL,
    review_note              TEXT         NULL,
    created_by_user_id       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_dealer_showcases_uuid UNIQUE (uuid),
    CONSTRAINT uq_dealer_showcases_org UNIQUE (organization_id),
    -- Target of the owner-consistent FKs of services and photos.
    CONSTRAINT uq_dealer_showcases_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_dealer_showcases_status
        CHECK (status IN ('draft', 'pending_review', 'published', 'rejected')),
    CONSTRAINT chk_dealer_showcases_content CHECK (dealer_showcase_content_valid(content)),
    CONSTRAINT chk_dealer_showcases_working_hours CHECK (jsonb_typeof(working_hours) = 'object'),
    CONSTRAINT chk_dealer_showcases_social_links CHECK (dealer_showcase_social_links_valid(social_links)),
    CONSTRAINT chk_dealer_showcases_seo_keywords CHECK (dealer_showcase_keywords_valid(seo_keywords)),
    CONSTRAINT chk_dealer_showcases_place_id
        CHECK (google_place_id IS NULL OR (btrim(google_place_id) <> '' AND google_place_id = btrim(google_place_id))),
    CONSTRAINT chk_dealer_showcases_rating CHECK (google_rating IS NULL OR google_rating BETWEEN 1.0 AND 5.0),
    CONSTRAINT chk_dealer_showcases_review_count CHECK (google_review_count IS NULL OR google_review_count >= 0),
    CONSTRAINT chk_dealer_showcases_rating_source
        CHECK (google_rating_source IS NULL OR google_rating_source IN ('places', 'manual')),
    -- A rating always carries its source and refresh time.
    CONSTRAINT chk_dealer_showcases_rating_set CHECK (
        (google_rating IS NULL AND google_review_count IS NULL)
        OR (google_rating IS NOT NULL AND google_rating_source IS NOT NULL AND google_rating_updated_at IS NOT NULL)
    ),
    CONSTRAINT chk_dealer_showcases_published_content
        CHECK (published_content IS NULL OR jsonb_typeof(published_content) = 'object'),
    CONSTRAINT chk_dealer_showcases_published_pair
        CHECK ((published_content IS NULL) = (published_at IS NULL)),
    CONSTRAINT chk_dealer_showcases_published
        CHECK (status <> 'published' OR published_content IS NOT NULL),
    CONSTRAINT chk_dealer_showcases_pending
        CHECK (status <> 'pending_review' OR submitted_at IS NOT NULL),
    CONSTRAINT chk_dealer_showcases_rejected
        CHECK (status <> 'rejected' OR (review_note IS NOT NULL AND btrim(review_note) <> '')),
    CONSTRAINT chk_dealer_showcases_review_note
        CHECK (review_note IS NULL OR char_length(review_note) <= 2000)
);

-- Review queue: brand + status, newest change first.
CREATE INDEX idx_dealer_showcases_brand_status ON dealer_showcases (brand_id, status, updated_at DESC, id);

CREATE TRIGGER trg_dealer_showcases_set_updated_at
    BEFORE UPDATE ON dealer_showcases
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The owner is a dealer or distributor of the row's brand and never changes.
CREATE FUNCTION dealer_showcases_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand BIGINT;
    org_type  TEXT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid OR NEW.organization_id <> OLD.organization_id OR NEW.brand_id <> OLD.brand_id THEN
            RAISE EXCEPTION 'dealer_showcases: owner of showcase % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;
    SELECT brand_id, type INTO org_brand, org_type FROM organizations WHERE id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'dealer_showcases: brand % does not match organization % (brand %)',
            NEW.brand_id, NEW.organization_id, org_brand
            USING ERRCODE = 'check_violation';
    END IF;
    IF org_type NOT IN ('dealer', 'distributor') THEN
        RAISE EXCEPTION 'dealer_showcases: organization % is a %, not a dealer or distributor',
            NEW.organization_id, org_type
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_dealer_showcases_check_org
    BEFORE INSERT OR UPDATE OF uuid, organization_id, brand_id ON dealer_showcases
    FOR EACH ROW
    EXECUTE FUNCTION dealer_showcases_check_org();

-- 3. dealer_showcase_services: services / packages shown on the page ----------
-- No price column: the showcase never shows a price, recommended price
-- included (F4 S10, F5 S8).
CREATE TABLE dealer_showcase_services (
    id              BIGSERIAL   PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    showcase_id     BIGINT      NOT NULL,
    organization_id BIGINT      NOT NULL,
    brand_id        BIGINT      NOT NULL,
    kind            VARCHAR(20) NOT NULL,
    category_id     BIGINT      NULL,
    title           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    description     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    sort_order      INTEGER     NOT NULL DEFAULT 0,
    visible         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_dealer_showcase_services_uuid UNIQUE (uuid),
    CONSTRAINT fk_dealer_showcase_services_showcase FOREIGN KEY (showcase_id, organization_id, brand_id)
        REFERENCES dealer_showcases (id, organization_id, brand_id) ON DELETE CASCADE,
    -- The catalog category belongs to the showcase's brand.
    CONSTRAINT fk_dealer_showcase_services_category FOREIGN KEY (category_id, brand_id)
        REFERENCES product_categories (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_dealer_showcase_services_kind CHECK (kind IN ('product_category', 'custom')),
    CONSTRAINT chk_dealer_showcase_services_category CHECK (
        (kind = 'product_category' AND category_id IS NOT NULL)
        OR (kind = 'custom' AND category_id IS NULL AND title <> '{}'::jsonb)
    ),
    CONSTRAINT chk_dealer_showcase_services_title CHECK (dealer_showcase_locale_texts_valid(title)),
    CONSTRAINT chk_dealer_showcase_services_description CHECK (dealer_showcase_locale_texts_valid(description))
);

CREATE UNIQUE INDEX uq_dealer_showcase_services_category
    ON dealer_showcase_services (showcase_id, category_id)
    WHERE category_id IS NOT NULL;
CREATE INDEX idx_dealer_showcase_services_order ON dealer_showcase_services (showcase_id, sort_order, id);
CREATE INDEX idx_dealer_showcase_services_category ON dealer_showcase_services (category_id)
    WHERE category_id IS NOT NULL;

CREATE TRIGGER trg_dealer_showcase_services_set_updated_at
    BEFORE UPDATE ON dealer_showcase_services
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 4. dealer_showcase_photos: gallery (showcase.max_photos, default 12, is
-- enforced by the usecase under the showcase row lock) ------------------------
CREATE TABLE dealer_showcase_photos (
    id                 BIGSERIAL    PRIMARY KEY,
    uuid               UUID         NOT NULL DEFAULT gen_random_uuid(),
    showcase_id        BIGINT       NOT NULL,
    organization_id    BIGINT       NOT NULL,
    brand_id           BIGINT       NOT NULL,
    storage_key        TEXT         NOT NULL,
    mime               VARCHAR(32)  NOT NULL,
    size_bytes         BIGINT       NOT NULL,
    sha256             CHAR(64)     NOT NULL,
    caption            JSONB        NOT NULL DEFAULT '{}'::jsonb,
    sort_order         INTEGER      NOT NULL DEFAULT 0,
    created_by_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_dealer_showcase_photos_uuid UNIQUE (uuid),
    CONSTRAINT uq_dealer_showcase_photos_storage_key UNIQUE (storage_key),
    -- The same image is not uploaded twice to one gallery.
    CONSTRAINT uq_dealer_showcase_photos_sha256 UNIQUE (showcase_id, sha256),
    CONSTRAINT fk_dealer_showcase_photos_showcase FOREIGN KEY (showcase_id, organization_id, brand_id)
        REFERENCES dealer_showcases (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_dealer_showcase_photos_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_dealer_showcase_photos_mime CHECK (mime IN ('image/jpeg', 'image/png', 'image/webp')),
    CONSTRAINT chk_dealer_showcase_photos_size CHECK (size_bytes > 0 AND size_bytes <= 20971520),
    CONSTRAINT chk_dealer_showcase_photos_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_dealer_showcase_photos_caption CHECK (dealer_showcase_locale_texts_valid(caption))
);

CREATE INDEX idx_dealer_showcase_photos_order ON dealer_showcase_photos (showcase_id, sort_order, id);

CREATE TRIGGER trg_dealer_showcase_photos_set_updated_at
    BEFORE UPDATE ON dealer_showcase_photos
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 5. Permissions (catalog append-only, sort_order MAX+10) ---------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT v.name, v.slug, 'dealer_showcase', v.scopes, false, false, v.description,
       m.max_sort + v.n * 10
FROM (VALUES
    (1, 'Read showcase', 'showcase.read', ARRAY['managed', 'subtree', 'brand', 'all']::text[],
     'Read the dealer showcase (profile, working hours, services, photos) of the organizations in scope.'),
    (2, 'Write showcase', 'showcase.write', ARRAY['managed', 'subtree', 'brand', 'all']::text[],
     'Edit the dealer showcase and submit it for publication.'),
    (3, 'Review showcases', 'platform.showcase.review', ARRAY['brand', 'all']::text[],
     'Approve or reject dealer showcases submitted for publication.')
) AS v (n, name, slug, scopes, description)
CROSS JOIN (SELECT COALESCE(MAX(sort_order), 0) AS max_sort FROM permissions) m
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'showcase.read', 'all'),
    ('super_admin', 'showcase.write', 'all'),
    ('super_admin', 'platform.showcase.review', 'all'),
    ('center_staff', 'showcase.read', 'brand'),
    ('center_staff', 'platform.showcase.review', 'brand'),
    ('center_social', 'showcase.read', 'brand'),
    ('center_social', 'platform.showcase.review', 'brand'),
    ('distributor_owner', 'showcase.read', 'subtree'),
    ('distributor_owner', 'showcase.write', 'subtree'),
    ('dealer_owner', 'showcase.read', 'managed'),
    ('dealer_owner', 'showcase.write', 'managed'),
    ('dealer_staff', 'showcase.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
