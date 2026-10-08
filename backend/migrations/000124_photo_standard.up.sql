-- TEC-498 (F5-07a): vehicle intake photo standard.

CREATE TABLE photo_angles (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    key                 VARCHAR(64) NOT NULL,
    name                JSONB       NOT NULL,
    hint                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    example_storage_key TEXT        NULL,
    required            BOOLEAN     NOT NULL DEFAULT true,
    sort_order          INT         NOT NULL DEFAULT 0,
    active              BOOLEAN     NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_photo_angles_uuid UNIQUE (uuid),
    CONSTRAINT uq_photo_angles_brand_key UNIQUE (brand_id, key),
    CONSTRAINT uq_photo_angles_id_brand UNIQUE (id, brand_id),
    CONSTRAINT uq_photo_angles_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_photo_angles_key CHECK (key ~ '^[a-z0-9][a-z0-9_-]{1,62}[a-z0-9]$'),
    CONSTRAINT chk_photo_angles_name CHECK (jsonb_typeof(name) = 'object' AND name <> '{}'::jsonb),
    CONSTRAINT chk_photo_angles_hint CHECK (jsonb_typeof(hint) = 'object'),
    CONSTRAINT chk_photo_angles_example_key CHECK (example_storage_key IS NULL OR btrim(example_storage_key) <> '')
);

CREATE INDEX idx_photo_angles_org_sort ON photo_angles (organization_id, active DESC, sort_order, id);

CREATE TRIGGER trg_photo_angles_set_updated_at
    BEFORE UPDATE ON photo_angles
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION photo_angles_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    owner_type  TEXT;
    owner_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND (
        NEW.organization_id <> OLD.organization_id OR NEW.brand_id <> OLD.brand_id
        OR NEW.key <> OLD.key OR NEW.uuid <> OLD.uuid
    ) THEN
        RAISE EXCEPTION 'photo_angles: identity of angle % cannot change', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;

    SELECT type, brand_id INTO owner_type, owner_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (owner_type <> 'center' OR owner_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'photo_angles: owner % must be the brand center', NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_photo_angles_check_row
    BEFORE INSERT OR UPDATE ON photo_angles
    FOR EACH ROW
    EXECUTE FUNCTION photo_angles_check_row();

CREATE TABLE photo_angle_overrides (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    angle_id            BIGINT      NOT NULL,
    required            BOOLEAN     NOT NULL,
    hidden              BOOLEAN     NOT NULL DEFAULT false,
    created_by_user_id  BIGINT      NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_photo_angle_overrides_uuid UNIQUE (uuid),
    CONSTRAINT uq_photo_angle_overrides_org_angle UNIQUE (organization_id, angle_id),
    CONSTRAINT fk_photo_angle_overrides_angle FOREIGN KEY (angle_id, brand_id)
        REFERENCES photo_angles (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_photo_angle_overrides_visible_required CHECK (NOT hidden OR required = false)
);

CREATE INDEX idx_photo_angle_overrides_angle ON photo_angle_overrides (angle_id);

CREATE TRIGGER trg_photo_angle_overrides_set_updated_at
    BEFORE UPDATE ON photo_angle_overrides
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION photo_angle_overrides_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    owner_type  TEXT;
    owner_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND (
        NEW.organization_id <> OLD.organization_id OR NEW.brand_id <> OLD.brand_id
        OR NEW.angle_id <> OLD.angle_id OR NEW.uuid <> OLD.uuid
    ) THEN
        RAISE EXCEPTION 'photo_angle_overrides: identity of override % cannot change', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;

    SELECT type, brand_id INTO owner_type, owner_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (owner_type NOT IN ('center', 'distributor', 'dealer') OR owner_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'photo_angle_overrides: owner % is outside brand %', NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_photo_angle_overrides_check_row
    BEFORE INSERT OR UPDATE ON photo_angle_overrides
    FOR EACH ROW
    EXECUTE FUNCTION photo_angle_overrides_check_row();

CREATE TABLE intake_photos (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID        NOT NULL DEFAULT gen_random_uuid(),
    service_id          BIGINT      NOT NULL,
    organization_id     BIGINT      NOT NULL,
    brand_id            BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    angle_id            BIGINT      NOT NULL,
    storage_key         TEXT        NOT NULL,
    mime                VARCHAR(64) NOT NULL,
    size                BIGINT      NOT NULL,
    sha256              CHAR(64)    NOT NULL,
    width               INT         NULL,
    height              INT         NULL,
    exif_taken_at       TIMESTAMPTZ NULL,
    exif_lat            NUMERIC(10,7) NULL,
    exif_lng            NUMERIC(10,7) NULL,
    exif_device         VARCHAR(255) NULL,
    uploaded_by         BIGINT      NULL REFERENCES users (id) ON DELETE RESTRICT,
    deleted_at          TIMESTAMPTZ NULL,
    deleted_by          BIGINT      NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_intake_photos_uuid UNIQUE (uuid),
    CONSTRAINT fk_intake_photos_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_intake_photos_angle FOREIGN KEY (angle_id, brand_id)
        REFERENCES photo_angles (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_intake_photos_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_intake_photos_mime CHECK (mime IN ('image/jpeg', 'image/png', 'image/webp', 'image/heic')),
    CONSTRAINT chk_intake_photos_size CHECK (size > 0 AND size <= 12582912),
    CONSTRAINT chk_intake_photos_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_intake_photos_dimensions CHECK (
        (width IS NULL AND height IS NULL) OR (width > 0 AND height > 0)
    ),
    CONSTRAINT chk_intake_photos_gps CHECK (
        (exif_lat IS NULL AND exif_lng IS NULL)
        OR (exif_lat BETWEEN -90 AND 90 AND exif_lng BETWEEN -180 AND 180)
    ),
    CONSTRAINT chk_intake_photos_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);

CREATE UNIQUE INDEX uq_intake_photos_active_service_angle
    ON intake_photos (service_id, angle_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_intake_photos_service ON intake_photos (service_id, created_at, id);
CREATE INDEX idx_intake_photos_angle ON intake_photos (angle_id);

DELETE FROM system_settings WHERE key = 'photo_standard_enabled';

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order) VALUES
    ('Manage photo standard', 'photo_standard.manage', 'photo_standard', ARRAY['brand','all']::text[], false, false,
     'Create and update central vehicle intake angle definitions.', (SELECT COALESCE(MAX(sort_order), 0) + 10 FROM permissions)),
    ('Override photo standard', 'photo_standard.override', 'photo_standard', ARRAY['subtree']::text[], false, false,
     'Set distributor or dealer required/hidden overrides for intake photo angles.', (SELECT COALESCE(MAX(sort_order), 0) + 20 FROM permissions))
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    module = EXCLUDED.module,
    scopes = EXCLUDED.scopes,
    is_sensitive = EXCLUDED.is_sensitive,
    super_admin_only = EXCLUDED.super_admin_only,
    description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'photo_standard.manage', 'all'),
    ('super_admin', 'photo_standard.override', 'subtree'),
    ('center_owner', 'photo_standard.manage', 'brand'),
    ('center_manager', 'photo_standard.manage', 'brand'),
    ('center_staff', 'photo_standard.manage', 'brand'),
    ('distributor_owner', 'photo_standard.override', 'subtree')
) AS g (role_slug, permission_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.permission_slug
ON CONFLICT DO NOTHING;
