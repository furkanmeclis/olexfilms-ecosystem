-- TEC-159 (F1-08a): customer schema, vehicles, customer permissions and
-- organization phones in E.164 (K11, K19, K20, K29; TEC-100 decisions 1-7).
--
-- A customer is a users row (K11) plus an optional customer_profiles row.
-- The profile is identity-level and global: one person served by several
-- dealers has one profile. Like consents/legal_texts (000037) it carries no
-- organization_id/brand_id; visibility comes from customer_organizations
-- (user x organization x brand), which the application filters with the
-- scope filter (K20).

-- 1. users: anonymized status and merge pointer (TEC-100 decision 2).
-- Anonymized and merged users are never deleted (20 FKs cascade from users).
ALTER TABLE users DROP CONSTRAINT chk_users_status;
ALTER TABLE users ADD CONSTRAINT chk_users_status
    CHECK (status IN ('active', 'disabled', 'pending', 'anonymized'));

ALTER TABLE users
    ADD COLUMN merged_into_user_id BIGINT NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_users_merged_not_self CHECK (merged_into_user_id IS NULL OR merged_into_user_id <> id);

CREATE INDEX idx_users_merged_into ON users (merged_into_user_id)
    WHERE merged_into_user_id IS NOT NULL;

-- 2. customer_profiles. National id (TC) and tax number are encrypted in the
-- application with AES-256-GCM (CUSTOMER_PII_KEY, platform/crypto PIIBox);
-- *_enc holds nonce|ciphertext, *_last4 is the mask shown in API responses
-- (TEC-100 decision 3). Ciphertext and mask are set and cleared together.
CREATE TABLE customer_profiles (
    user_id             BIGINT       PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    type                VARCHAR(16)  NOT NULL DEFAULT 'individual',
    company_name        VARCHAR(200) NULL,
    tax_office          VARCHAR(150) NULL,
    national_id_enc     BYTEA        NULL,
    national_id_last4   VARCHAR(4)   NULL,
    tax_no_enc          BYTEA        NULL,
    tax_no_last4        VARCHAR(4)   NULL,
    address             JSONB        NOT NULL DEFAULT '{}'::jsonb,
    notification_prefs  JSONB        NOT NULL DEFAULT '{"whatsapp": true, "email": true, "sms": false, "push": true}'::jsonb,
    anonymized_at       TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_customer_profiles_type CHECK (type IN ('individual', 'corporate')),
    CONSTRAINT chk_customer_profiles_national_id CHECK ((national_id_enc IS NULL) = (national_id_last4 IS NULL)),
    CONSTRAINT chk_customer_profiles_tax_no CHECK ((tax_no_enc IS NULL) = (tax_no_last4 IS NULL)),
    CONSTRAINT chk_customer_profiles_national_id_last4 CHECK (national_id_last4 IS NULL OR national_id_last4 ~ '^[0-9A-Z]{1,4}$'),
    CONSTRAINT chk_customer_profiles_tax_no_last4 CHECK (tax_no_last4 IS NULL OR tax_no_last4 ~ '^[0-9A-Z]{1,4}$'),
    CONSTRAINT chk_customer_profiles_address CHECK (jsonb_typeof(address) = 'object'),
    CONSTRAINT chk_customer_profiles_notification_prefs CHECK (jsonb_typeof(notification_prefs) = 'object')
);

CREATE TRIGGER trg_customer_profiles_set_updated_at
    BEFORE UPDATE ON customer_profiles
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- brand_id of a scoped row must be the brand of its organization (K1/K20).
CREATE FUNCTION customer_scope_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand BIGINT;
BEGIN
    IF NEW.organization_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT brand_id INTO org_brand FROM organizations WHERE id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION '%: brand % does not match organization % (brand %)',
            TG_TABLE_NAME, NEW.brand_id, NEW.organization_id, org_brand
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

-- 3. customer_organizations: which organization serves which customer. A
-- row is added when a dealer creates the customer or opens a service; a
-- dealer sees only customers linked to the organizations of its scope.
CREATE TABLE customer_organizations (
    id               BIGSERIAL    PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    first_service_at TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_organizations_user_org UNIQUE (user_id, organization_id)
);

CREATE INDEX idx_customer_organizations_org ON customer_organizations (organization_id, created_at DESC);
CREATE INDEX idx_customer_organizations_brand_user ON customer_organizations (brand_id, user_id);

CREATE TRIGGER trg_customer_organizations_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON customer_organizations
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. vehicles. Owned by a user; organization_id is the organization that
-- registered it (NULL when the customer added it), brand_id the domain
-- brand (K20). A plate can change hands and be re-issued, so there is no
-- unique key on (plate_country, plate_normalized), only a lookup index;
-- plate_normalized is geo.NormalizePlate(plate). VIN is optional, 17
-- characters without I/O/Q, and not unique either (ownership transfer is
-- F1-06; duplicates are warned about in the application).
CREATE TABLE vehicles (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id           BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    organization_id   BIGINT       NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id          BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    car_brand_id      BIGINT       NULL REFERENCES car_brands (id) ON DELETE RESTRICT,
    car_model_id      BIGINT       NULL REFERENCES car_models (id) ON DELETE RESTRICT,
    model_year        SMALLINT     NULL,
    plate             VARCHAR(20)  NULL,
    plate_normalized  VARCHAR(20)  NULL,
    plate_country     CHAR(2)      NULL,
    vin               VARCHAR(17)  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ  NULL,
    CONSTRAINT uq_vehicles_uuid UNIQUE (uuid),
    CONSTRAINT chk_vehicles_model_needs_brand CHECK (car_model_id IS NULL OR car_brand_id IS NOT NULL),
    CONSTRAINT chk_vehicles_model_year CHECK (model_year IS NULL OR model_year BETWEEN 1900 AND 2100),
    CONSTRAINT chk_vehicles_plate CHECK (
        (plate IS NULL AND plate_normalized IS NULL)
        OR (plate IS NOT NULL AND plate_normalized IS NOT NULL AND plate_country IS NOT NULL
            AND btrim(plate) <> '' AND plate_normalized <> '')
    ),
    CONSTRAINT chk_vehicles_plate_country CHECK (plate_country IS NULL OR plate_country ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_vehicles_vin CHECK (vin IS NULL OR vin ~ '^[A-HJ-NPR-Z0-9]{17}$')
);

CREATE INDEX idx_vehicles_user ON vehicles (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_vehicles_plate ON vehicles (plate_country, plate_normalized)
    WHERE deleted_at IS NULL AND plate_normalized IS NOT NULL;
CREATE INDEX idx_vehicles_vin ON vehicles (vin) WHERE deleted_at IS NULL AND vin IS NOT NULL;
CREATE INDEX idx_vehicles_org ON vehicles (organization_id) WHERE organization_id IS NOT NULL;

CREATE TRIGGER trg_vehicles_set_updated_at
    BEFORE UPDATE ON vehicles
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_vehicles_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON vehicles
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- The model must belong to the car brand.
CREATE FUNCTION vehicles_check_model() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    model_brand BIGINT;
BEGIN
    IF NEW.car_model_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT car_brand_id INTO model_brand FROM car_models WHERE id = NEW.car_model_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF model_brand IS DISTINCT FROM NEW.car_brand_id THEN
        RAISE EXCEPTION 'vehicles: car model % belongs to car brand %, not %',
            NEW.car_model_id, model_brand, NEW.car_brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_vehicles_check_model
    BEFORE INSERT OR UPDATE OF car_brand_id, car_model_id ON vehicles
    FOR EACH ROW
    EXECUTE FUNCTION vehicles_check_model();

-- 5. Organization phone in E.164 (K29, TEC-84 follow-up). Converting a
-- free-text number needs libphonenumber and the organization's country, so
-- SQL only moves every non-E.164 value aside into phone_raw (nothing is
-- deleted) and the idempotent `cmd/normalize-org-phones` command parses it
-- back (success: phone = E.164, phone_raw = NULL; failure: kept in
-- phone_raw and reported). New writes are normalized in the organizations
-- usecase; the CHECK enforces it.
ALTER TABLE organizations ADD COLUMN phone_raw VARCHAR(32) NULL;

UPDATE organizations
SET phone_raw = phone, phone = ''
WHERE btrim(phone) <> '' AND phone !~ '^\+[1-9][0-9]{6,14}$';

UPDATE organizations SET phone = '' WHERE phone <> '' AND btrim(phone) = '';

ALTER TABLE organizations ADD CONSTRAINT chk_organizations_phone_e164
    CHECK (phone = '' OR phone ~ '^\+[1-9][0-9]{6,14}$');

-- 6. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last, so sort_order continues after the current maximum).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read vehicles', 'vehicles.read', 'vehicles',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all', 'customer']::text[], false, false,
       'Customer vehicles: plate, VIN, car brand and model.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write vehicles', 'vehicles.write', 'vehicles',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Create and edit customer vehicles.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Anonymize customers', 'customers.anonymize', 'customers',
       ARRAY['brand', 'all']::text[], true, false,
       'KVKK/GDPR anonymization of a customer; services and warranties stay (center only, K19).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Merge customers', 'customers.merge', 'customers',
       ARRAY['brand', 'all']::text[], true, false,
       'Merge duplicate customer accounts into one user (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'vehicles.read', 'all'),
    ('super_admin', 'vehicles.write', 'all'),
    ('super_admin', 'customers.anonymize', 'all'),
    ('super_admin', 'customers.merge', 'all'),
    ('center_staff', 'vehicles.read', 'brand'),
    ('center_staff', 'vehicles.write', 'brand'),
    ('center_staff', 'customers.anonymize', 'brand'),
    ('center_staff', 'customers.merge', 'brand'),
    ('center_social', 'vehicles.read', 'brand'),
    ('distributor_owner', 'vehicles.read', 'subtree'),
    ('distributor_owner', 'vehicles.write', 'subtree'),
    ('distributor_staff', 'vehicles.read', 'subtree'),
    ('distributor_staff', 'vehicles.write', 'subtree'),
    ('dealer_owner', 'vehicles.read', 'managed'),
    ('dealer_owner', 'vehicles.write', 'managed'),
    ('dealer_staff', 'vehicles.read', 'managed'),
    ('dealer_staff', 'vehicles.write', 'own'),
    ('customer', 'vehicles.read', 'customer'),
    ('fleet', 'vehicles.read', 'customer')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
