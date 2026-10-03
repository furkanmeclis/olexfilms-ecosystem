-- TEC-244 (F2-03h): the service review form of the customer portal.
--
-- The portal user who owns a completed service (its customer or a holder of
-- one of its warranties) rates the platform and the product quality once:
-- one row per service (uq_service_reviews_service). customer_user_id is the
-- user who sent the form. Processing and reporting come with F5; this table
-- only stores the answers. The Google review request (TEC-192) stays a
-- separate flow on organizations.google_business_url.

CREATE TABLE service_reviews (
    id                BIGSERIAL     PRIMARY KEY,
    uuid              UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id          BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    service_id        BIGINT        NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    customer_user_id  BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    platform_rating   SMALLINT      NOT NULL,
    product_rating    SMALLINT      NOT NULL,
    comment           VARCHAR(2000) NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_reviews_uuid UNIQUE (uuid),
    CONSTRAINT uq_service_reviews_service UNIQUE (service_id),
    CONSTRAINT chk_service_reviews_platform_rating CHECK (platform_rating BETWEEN 1 AND 5),
    CONSTRAINT chk_service_reviews_product_rating CHECK (product_rating BETWEEN 1 AND 5),
    CONSTRAINT chk_service_reviews_comment CHECK (comment IS NULL OR btrim(comment) <> '')
);

CREATE INDEX idx_service_reviews_org_created ON service_reviews (organization_id, created_at DESC);
CREATE INDEX idx_service_reviews_customer ON service_reviews (customer_user_id);

-- brand_id must be the brand of the organization (000048 helper).
CREATE TRIGGER trg_service_reviews_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON service_reviews
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();
