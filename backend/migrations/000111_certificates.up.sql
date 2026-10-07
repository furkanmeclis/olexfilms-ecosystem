-- TEC-479 (F5-03a): certificate schema, permissions and sqlc.
-- API and service policy land in F5-03b; this migration creates the scoped
-- catalog, user certificates and per-service warning queue.

-- 1. Certificate type catalog ---------------------------------------------
CREATE TABLE certificate_types (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name             JSONB       NOT NULL,
    description      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    validity_months  INT         NULL,
    active           BOOLEAN     NOT NULL DEFAULT true,
    sort_order       INT         NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_certificate_types_uuid UNIQUE (uuid),
    CONSTRAINT uq_certificate_types_id_brand UNIQUE (id, brand_id),
    CONSTRAINT chk_certificate_types_name CHECK (jsonb_typeof(name) = 'object' AND name <> '{}'::jsonb),
    CONSTRAINT chk_certificate_types_description CHECK (jsonb_typeof(description) = 'object'),
    CONSTRAINT chk_certificate_types_validity CHECK (validity_months IS NULL OR validity_months > 0)
);

CREATE INDEX idx_certificate_types_brand_active ON certificate_types (brand_id, active, sort_order, id);

CREATE TRIGGER trg_certificate_types_set_updated_at
    BEFORE UPDATE ON certificate_types
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_certificate_types_center_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON certificate_types
    FOR EACH ROW
    EXECUTE FUNCTION catalog_check_center_org();

-- 2. Certificate type bindings --------------------------------------------
CREATE TABLE certificate_type_categories (
    type_id          BIGINT      NOT NULL,
    organization_id  BIGINT      NOT NULL,
    brand_id         BIGINT      NOT NULL,
    category_id      BIGINT      NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (type_id, category_id),
    CONSTRAINT fk_certificate_type_categories_type FOREIGN KEY (type_id, brand_id)
        REFERENCES certificate_types (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_certificate_type_categories_category FOREIGN KEY (category_id, brand_id)
        REFERENCES product_categories (id, brand_id) ON DELETE RESTRICT
);

CREATE INDEX idx_certificate_type_categories_category ON certificate_type_categories (category_id, type_id);

CREATE TRIGGER trg_certificate_type_categories_center_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON certificate_type_categories
    FOR EACH ROW
    EXECUTE FUNCTION catalog_check_center_org();

CREATE TABLE certificate_type_products (
    type_id          BIGINT      NOT NULL,
    organization_id  BIGINT      NOT NULL,
    brand_id         BIGINT      NOT NULL,
    product_id       BIGINT      NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (type_id, product_id),
    CONSTRAINT fk_certificate_type_products_type FOREIGN KEY (type_id, brand_id)
        REFERENCES certificate_types (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_certificate_type_products_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT
);

CREATE INDEX idx_certificate_type_products_product ON certificate_type_products (product_id, type_id);

CREATE TRIGGER trg_certificate_type_products_center_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON certificate_type_products
    FOR EACH ROW
    EXECUTE FUNCTION catalog_check_center_org();

-- 3. User certificates ------------------------------------------------------
CREATE TABLE certificates (
    id                    BIGSERIAL PRIMARY KEY,
    uuid                  UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id               BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    organization_id       BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    type_id               BIGINT      NOT NULL,
    storage_key           TEXT        NOT NULL,
    sha256                CHAR(64)    NOT NULL,
    issued_at             TIMESTAMPTZ NOT NULL,
    expires_at            TIMESTAMPTZ NULL,
    status                TEXT        NOT NULL DEFAULT 'pending',
    verified_by_user_id   BIGINT      NULL REFERENCES users (id) ON DELETE RESTRICT,
    verified_by_org_id    BIGINT      NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    verified_at           TIMESTAMPTZ NULL,
    reject_reason         TEXT        NULL,
    expiry_notice_sent_at TIMESTAMPTZ NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_certificates_uuid UNIQUE (uuid),
    CONSTRAINT fk_certificates_type FOREIGN KEY (type_id, brand_id)
        REFERENCES certificate_types (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_certificates_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_certificates_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_certificates_status CHECK (status IN ('pending', 'valid', 'rejected', 'expired', 'revoked')),
    CONSTRAINT chk_certificates_period CHECK (expires_at IS NULL OR expires_at > issued_at),
    CONSTRAINT chk_certificates_verified CHECK (
        (status IN ('valid', 'rejected') AND verified_by_user_id IS NOT NULL
            AND verified_by_org_id IS NOT NULL AND verified_at IS NOT NULL)
        OR (status NOT IN ('valid', 'rejected') AND verified_at IS NULL)
    ),
    CONSTRAINT chk_certificates_reject_reason CHECK (
        (status = 'rejected') = (reject_reason IS NOT NULL AND btrim(reject_reason) <> '')
    )
);

CREATE UNIQUE INDEX uq_certificates_user_type_sha ON certificates (user_id, type_id, sha256);
CREATE INDEX idx_certificates_org_status_expires ON certificates (organization_id, status, expires_at, id);
CREATE INDEX idx_certificates_brand_type ON certificates (brand_id, type_id, status);
CREATE INDEX idx_certificates_user ON certificates (user_id, status, expires_at);
CREATE INDEX idx_certificates_expiry_notice ON certificates (expires_at, id)
    WHERE status = 'valid' AND expiry_notice_sent_at IS NULL AND expires_at IS NOT NULL;

CREATE TRIGGER trg_certificates_set_updated_at
    BEFORE UPDATE ON certificates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_certificates_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON certificates
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE FUNCTION certificates_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    cert_type_brand BIGINT;
    verifier_brand  BIGINT;
BEGIN
    SELECT brand_id INTO cert_type_brand FROM certificate_types WHERE id = NEW.type_id;
    IF FOUND AND cert_type_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'certificates: type % belongs to brand %, not %',
            NEW.type_id, cert_type_brand, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.verified_by_org_id IS NOT NULL THEN
        SELECT brand_id INTO verifier_brand FROM organizations WHERE id = NEW.verified_by_org_id;
        IF FOUND AND verifier_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'certificates: verifier org % belongs to brand %, not %',
                NEW.verified_by_org_id, verifier_brand, NEW.brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_certificates_check_row
    BEFORE INSERT OR UPDATE OF brand_id, type_id, verified_by_org_id ON certificates
    FOR EACH ROW
    EXECUTE FUNCTION certificates_check_row();

-- 4. Service certificate warnings ------------------------------------------
CREATE TABLE service_certificate_warnings (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    service_id      BIGINT      NOT NULL,
    organization_id BIGINT      NOT NULL,
    brand_id        BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    type_id         BIGINT      NOT NULL,
    reason          TEXT        NOT NULL,
    decision        TEXT        NOT NULL DEFAULT 'none',
    decided_by      BIGINT      NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at      TIMESTAMPTZ NULL,
    note            TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_certificate_warnings_uuid UNIQUE (uuid),
    CONSTRAINT uq_service_certificate_warnings_service_user_type UNIQUE (service_id, user_id, type_id),
    CONSTRAINT fk_service_certificate_warnings_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_service_certificate_warnings_type FOREIGN KEY (type_id, brand_id)
        REFERENCES certificate_types (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_certificate_warnings_reason CHECK (reason IN ('missing', 'expired', 'pending')),
    CONSTRAINT chk_service_certificate_warnings_decision CHECK (decision IN ('none', 'pending_approval', 'approved', 'rejected')),
    CONSTRAINT chk_service_certificate_warnings_decided CHECK (
        (decision IN ('approved', 'rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL)
        OR (decision NOT IN ('approved', 'rejected') AND decided_by IS NULL AND decided_at IS NULL)
    ),
    CONSTRAINT chk_service_certificate_warnings_note CHECK (char_length(note) <= 5000)
);

CREATE INDEX idx_service_certificate_warnings_queue
    ON service_certificate_warnings (brand_id, decision, created_at, id);
CREATE INDEX idx_service_certificate_warnings_org
    ON service_certificate_warnings (organization_id, decision, created_at, id);

CREATE TRIGGER trg_service_certificate_warnings_set_updated_at
    BEFORE UPDATE ON service_certificate_warnings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_service_certificate_warnings_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON service_certificate_warnings
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage certificate types', 'certificate_types.manage', 'certificates',
       ARRAY['brand', 'all']::text[], false, false,
       'Create and update certificate types and their product/category bindings.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read certificates', 'certificates.read', 'certificates',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read user certificates and service certificate warnings in scope.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write certificates', 'certificates.write', 'certificates',
       ARRAY['managed', 'subtree', 'all']::text[], false, false,
       'Upload certificates for staff in the managed organization or subtree.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Verify certificates', 'certificates.verify', 'certificates',
       ARRAY['subtree', 'brand', 'all']::text[], false, false,
       'Verify or reject certificates of the brand or distributor subtree.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Approve certificate service warnings', 'certificates.approve_service', 'certificates',
       ARRAY['brand', 'all']::text[], false, false,
       'Approve or reject service status changes blocked by certificate warnings.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'certificate_types.manage', 'all'),
    ('super_admin', 'certificates.read', 'all'),
    ('super_admin', 'certificates.write', 'all'),
    ('super_admin', 'certificates.verify', 'all'),
    ('super_admin', 'certificates.approve_service', 'all'),
    ('center_staff', 'certificate_types.manage', 'brand'),
    ('center_staff', 'certificates.read', 'brand'),
    ('center_staff', 'certificates.verify', 'brand'),
    ('center_staff', 'certificates.approve_service', 'brand'),
    ('distributor_owner', 'certificates.read', 'subtree'),
    ('distributor_owner', 'certificates.write', 'subtree'),
    ('distributor_owner', 'certificates.verify', 'subtree'),
    ('dealer_owner', 'certificates.read', 'managed'),
    ('dealer_owner', 'certificates.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
