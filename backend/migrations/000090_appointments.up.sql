-- TEC-322 (F3-04a): appointment schema, permissions and sqlc.
-- API endpoints and capacity search land in TEC-323/324; this migration
-- fixes the data model and database guards.

-- 1. Organization appointment settings ------------------------------------
CREATE TABLE appointment_settings (
    id                          BIGSERIAL    PRIMARY KEY,
    uuid                        UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id             BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                    BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    daily_vehicle_capacity      INTEGER      NOT NULL,
    default_estimated_minutes   INTEGER      NOT NULL,
    slot_interval_minutes       INTEGER      NOT NULL,
    working_hours               JSONB        NOT NULL DEFAULT '{}'::jsonb,
    portal_appointments_enabled BOOLEAN      NOT NULL DEFAULT false,
    created_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_appointment_settings_uuid UNIQUE (uuid),
    CONSTRAINT uq_appointment_settings_org UNIQUE (organization_id),
    CONSTRAINT uq_appointment_settings_org_brand UNIQUE (organization_id, brand_id),
    CONSTRAINT chk_appointment_settings_capacity CHECK (daily_vehicle_capacity > 0),
    CONSTRAINT chk_appointment_settings_default_duration CHECK (default_estimated_minutes > 0),
    CONSTRAINT chk_appointment_settings_slot_interval CHECK (slot_interval_minutes > 0),
    CONSTRAINT chk_appointment_settings_working_hours CHECK (jsonb_typeof(working_hours) = 'object')
);

CREATE TRIGGER trg_appointment_settings_set_updated_at
    BEFORE UPDATE ON appointment_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_appointment_settings_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON appointment_settings
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE FUNCTION appointment_settings_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id THEN
            RAISE EXCEPTION 'appointment_settings: owner of settings % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_appointment_settings_check_row
    BEFORE INSERT OR UPDATE ON appointment_settings
    FOR EACH ROW
    EXECUTE FUNCTION appointment_settings_check_row();

-- 2. Closure days ----------------------------------------------------------
CREATE TABLE appointment_closures (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    closed_on        DATE         NOT NULL,
    reason           TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_appointment_closures_uuid UNIQUE (uuid),
    CONSTRAINT uq_appointment_closures_org_day UNIQUE (organization_id, closed_on),
    CONSTRAINT chk_appointment_closures_reason CHECK (char_length(reason) <= 1000)
);

CREATE INDEX idx_appointment_closures_org_day ON appointment_closures (organization_id, closed_on);

CREATE TRIGGER trg_appointment_closures_set_updated_at
    BEFORE UPDATE ON appointment_closures
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_appointment_closures_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON appointment_closures
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 3. Appointments ----------------------------------------------------------
CREATE TABLE appointments (
    id                         BIGSERIAL    PRIMARY KEY,
    uuid                       UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id            BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                   BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    customer_user_id           BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    vehicle_id                 BIGINT       NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    starts_at                  TIMESTAMPTZ  NOT NULL,
    ends_at                    TIMESTAMPTZ  NOT NULL,
    estimated_minutes          INTEGER      NOT NULL,
    source                     VARCHAR(16)  NOT NULL,
    status                     VARCHAR(16)  NOT NULL DEFAULT 'scheduled',
    cancel_reason              TEXT         NULL,
    lead_id                    BIGINT       NULL,
    service_id                 BIGINT       NULL,
    note                       TEXT         NOT NULL DEFAULT '',
    created_by_user_id         BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    reminded_24h_at            TIMESTAMPTZ  NULL,
    reminded_2h_at             TIMESTAMPTZ  NULL,
    created_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at                 TIMESTAMPTZ  NULL,
    CONSTRAINT uq_appointments_uuid UNIQUE (uuid),
    CONSTRAINT uq_appointments_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_appointments_lead FOREIGN KEY (lead_id, organization_id, brand_id)
        REFERENCES leads (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_appointments_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_appointments_time CHECK (ends_at > starts_at),
    CONSTRAINT chk_appointments_estimated_minutes CHECK (estimated_minutes > 0),
    CONSTRAINT chk_appointments_source CHECK (source IN ('panel', 'portal', 'assistant', 'lead')),
    CONSTRAINT chk_appointments_status CHECK (
        status IN ('scheduled', 'confirmed', 'arrived', 'no_show', 'cancelled')),
    CONSTRAINT chk_appointments_cancel_reason CHECK (
        (status = 'cancelled') = (cancel_reason IS NOT NULL)),
    CONSTRAINT chk_appointments_note CHECK (char_length(note) <= 20000)
);

CREATE INDEX idx_appointments_org_starts_at ON appointments (organization_id, starts_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_appointments_customer_starts_at ON appointments (customer_user_id, starts_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_appointments_lead ON appointments (lead_id) WHERE lead_id IS NOT NULL;
CREATE INDEX idx_appointments_service ON appointments (service_id) WHERE service_id IS NOT NULL;

CREATE TRIGGER trg_appointments_set_updated_at
    BEFORE UPDATE ON appointments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_appointments_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON appointments
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE FUNCTION appointments_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_brand BIGINT;
    v_user  BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id THEN
            RAISE EXCEPTION 'appointments: owner of appointment % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF NEW.vehicle_id IS NOT NULL THEN
        SELECT brand_id, user_id INTO v_brand, v_user FROM vehicles WHERE id = NEW.vehicle_id;
        IF FOUND THEN
            IF v_brand IS DISTINCT FROM NEW.brand_id THEN
                RAISE EXCEPTION 'appointments: vehicle % is outside brand %', NEW.vehicle_id, NEW.brand_id
                    USING ERRCODE = 'check_violation';
            END IF;
            IF v_user IS DISTINCT FROM NEW.customer_user_id THEN
                RAISE EXCEPTION 'appointments: vehicle % does not belong to customer %',
                    NEW.vehicle_id, NEW.customer_user_id
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_appointments_check_row
    BEFORE INSERT OR UPDATE ON appointments
    FOR EACH ROW
    EXECUTE FUNCTION appointments_check_row();

-- 4. Permissions -----------------------------------------------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read appointments', 'appointments.read', 'appointments',
       ARRAY['managed', 'subtree', 'all']::text[], false, false,
       'Read appointment calendars and capacity of managed organizations.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write appointments', 'appointments.write', 'appointments',
       ARRAY['managed', 'all']::text[], false, false,
       'Create, update and cancel appointments of the managed organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage appointment settings', 'appointment_settings.manage', 'appointments',
       ARRAY['managed', 'all']::text[], false, false,
       'Manage appointment capacity, working hours, closures and portal booking settings.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'appointments.read', 'all'),
    ('super_admin', 'appointments.write', 'all'),
    ('super_admin', 'appointment_settings.manage', 'all'),
    ('center_staff', 'appointments.read', 'all'),
    ('center_staff', 'appointments.write', 'managed'),
    ('center_staff', 'appointment_settings.manage', 'managed'),
    ('center_social', 'appointments.read', 'all'),
    ('center_social', 'appointments.write', 'managed'),
    ('distributor_owner', 'appointments.read', 'subtree'),
    ('distributor_owner', 'appointments.write', 'managed'),
    ('distributor_owner', 'appointment_settings.manage', 'managed'),
    ('distributor_staff', 'appointments.read', 'subtree'),
    ('distributor_staff', 'appointments.write', 'managed'),
    ('dealer_owner', 'appointments.read', 'managed'),
    ('dealer_owner', 'appointments.write', 'managed'),
    ('dealer_owner', 'appointment_settings.manage', 'managed'),
    ('dealer_staff', 'appointments.read', 'managed'),
    ('dealer_staff', 'appointments.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
