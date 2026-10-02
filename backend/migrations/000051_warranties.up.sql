-- TEC-185 (F1-06a): warranty schema, vehicle ownership transfers, the
-- organization's Google Business link and the warranty permissions.
-- Decisions: TEC-98 orchestrator comment items 1-8.
--
--   * warranties: one row per completed service item (UNIQUE
--     service_item_id is both the duplicate guard and the idempotency key of
--     the service.completed consumer, decision 3). organization_id is the
--     organization that performed the service, brand_id its brand.
--     holder_user_id starts as the service customer and moves with a vehicle
--     transfer (decision 6). public_code is a random, URL-safe code printed
--     in the QR (/garanti/{public_code}, decision 1); service and warranty
--     numbers are sequential and never used publicly.
--   * item_kind is a snapshot of service_items.kind, enforced by a composite
--     FK so it cannot drift. Full units: one active warranty per (vehicle,
--     unit) (partial unique index). Partial cuts of the same roll each get
--     their own warranty and are outside that rule (decision 3).
--   * start_at / end_at are instants. The use case computes end_at as the
--     end of the last covered day in the organization's time zone
--     (start + N calendar months, clamped to the month end, decision 4), so
--     the expiry cron only compares end_at with NOW().
--   * vehicle_transfers: vehicle-based ownership transfer with two hashed
--     6-digit codes (current owner and new owner, sent over WhatsApp);
--     codes are never stored in clear text.
--   * organizations.google_business_url: review request link (decision 7).

-- 1. Service items: target of the warranty FK, so a warranty always copies
-- the item's service, organization, brand, product, unit and kind.
ALTER TABLE service_items
    ADD CONSTRAINT uq_service_items_warranty_ref
        UNIQUE (id, service_id, organization_id, brand_id, product_id, unit_id, kind);

-- 2. Warranties.
CREATE TABLE warranties (
    id                 BIGSERIAL     PRIMARY KEY,
    uuid               UUID          NOT NULL DEFAULT gen_random_uuid(),
    -- 22 URL-safe characters from 16 random bytes of gen_random_uuid().
    public_code        VARCHAR(32)   NOT NULL DEFAULT rtrim(
                           translate(encode(uuid_send(gen_random_uuid()), 'base64'), '+/', '-_'), '='),
    organization_id    BIGINT        NOT NULL,
    brand_id           BIGINT        NOT NULL,
    service_id         BIGINT        NOT NULL,
    service_item_id    BIGINT        NOT NULL,
    product_id         BIGINT        NOT NULL,
    unit_id            BIGINT        NOT NULL,
    item_kind          VARCHAR(16)   NOT NULL,
    vehicle_id         BIGINT        NOT NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    holder_user_id     BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    start_at           TIMESTAMPTZ   NOT NULL,
    end_at             TIMESTAMPTZ   NOT NULL,
    status             VARCHAR(16)   NOT NULL DEFAULT 'active',
    expired_at         TIMESTAMPTZ   NULL,
    voided_at          TIMESTAMPTZ   NULL,
    voided_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    void_reason        TEXT          NULL,
    notified_30_at     TIMESTAMPTZ   NULL,
    notified_7_at      TIMESTAMPTZ   NULL,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warranties_uuid UNIQUE (uuid),
    CONSTRAINT uq_warranties_public_code UNIQUE (public_code),
    -- One warranty per service item (decision 3, consumer idempotency).
    CONSTRAINT uq_warranties_service_item UNIQUE (service_item_id),
    CONSTRAINT fk_warranties_service FOREIGN KEY (service_id, organization_id, brand_id)
        REFERENCES services (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_warranties_service_item
        FOREIGN KEY (service_item_id, service_id, organization_id, brand_id, product_id, unit_id, item_kind)
        REFERENCES service_items (id, service_id, organization_id, brand_id, product_id, unit_id, kind)
        ON DELETE RESTRICT,
    CONSTRAINT chk_warranties_public_code CHECK (public_code ~ '^[A-Za-z0-9_-]{12,32}$'),
    CONSTRAINT chk_warranties_item_kind CHECK (item_kind IN ('full', 'partial')),
    CONSTRAINT chk_warranties_period CHECK (end_at > start_at),
    CONSTRAINT chk_warranties_status CHECK (status IN ('active', 'expired', 'void')),
    CONSTRAINT chk_warranties_expired CHECK ((status = 'expired') = (expired_at IS NOT NULL)),
    CONSTRAINT chk_warranties_voided CHECK ((status = 'void') = (voided_at IS NOT NULL))
);

-- Full units: one active warranty per vehicle and unit (decision 3).
CREATE UNIQUE INDEX uq_warranties_active_full_unit ON warranties (vehicle_id, unit_id)
    WHERE status = 'active' AND item_kind = 'full';

CREATE INDEX idx_warranties_service ON warranties (service_id, id);
CREATE INDEX idx_warranties_org_created ON warranties (organization_id, created_at);
CREATE INDEX idx_warranties_brand_status ON warranties (brand_id, status, end_at);
CREATE INDEX idx_warranties_holder ON warranties (holder_user_id, status);
CREATE INDEX idx_warranties_vehicle ON warranties (vehicle_id, status);
CREATE INDEX idx_warranties_unit ON warranties (unit_id);
-- Expiry and reminder cron (decision 5).
CREATE INDEX idx_warranties_active_end ON warranties (end_at) WHERE status = 'active';

CREATE TRIGGER trg_warranties_set_updated_at
    BEFORE UPDATE ON warranties
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- A warranty is opened from a completed service and covers that service's
-- vehicle. The service link, the covered item, the vehicle, the period
-- start and the public code never change; void is final and an expired
-- warranty only becomes void. Holder, end_at (extension) and notification
-- stamps stay writable.
CREATE FUNCTION warranties_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    svc_status  VARCHAR(16);
    svc_vehicle BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT status, vehicle_id INTO svc_status, svc_vehicle FROM services WHERE id = NEW.service_id;
        IF NOT FOUND THEN
            RETURN NEW; -- the foreign key reports it
        END IF;
        IF svc_status IS DISTINCT FROM 'completed' THEN
            RAISE EXCEPTION 'warranties: service % is %, not completed', NEW.service_id, svc_status
                USING ERRCODE = 'check_violation';
        END IF;
        IF svc_vehicle IS DISTINCT FROM NEW.vehicle_id THEN
            RAISE EXCEPTION 'warranties: vehicle % is not the vehicle of service %', NEW.vehicle_id, NEW.service_id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.service_id IS DISTINCT FROM OLD.service_id
       OR NEW.service_item_id IS DISTINCT FROM OLD.service_item_id
       OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
       OR NEW.brand_id IS DISTINCT FROM OLD.brand_id
       OR NEW.product_id IS DISTINCT FROM OLD.product_id
       OR NEW.unit_id IS DISTINCT FROM OLD.unit_id
       OR NEW.item_kind IS DISTINCT FROM OLD.item_kind
       OR NEW.vehicle_id IS DISTINCT FROM OLD.vehicle_id
       OR NEW.start_at IS DISTINCT FROM OLD.start_at
       OR NEW.public_code IS DISTINCT FROM OLD.public_code THEN
        RAISE EXCEPTION 'warranties: the covered item, period start and public code are immutable (warranty %)', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status
       AND NOT (OLD.status = 'active' OR (OLD.status = 'expired' AND NEW.status = 'void')) THEN
        RAISE EXCEPTION 'warranties: status % cannot change to % (warranty %)', OLD.status, NEW.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_warranties_check_row
    BEFORE INSERT OR UPDATE ON warranties
    FOR EACH ROW
    EXECUTE FUNCTION warranties_check_row();

-- 3. Vehicle ownership transfers (decision 6). organization_id / brand_id:
-- the organization that started the transfer (the vehicle's dealer). The
-- new owner is reached by phone (E.164, K29); to_user_id is set once the
-- user is resolved or created, at the latest on completion.
CREATE TABLE vehicle_transfers (
    id                    BIGSERIAL     PRIMARY KEY,
    uuid                  UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    vehicle_id            BIGINT        NOT NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    from_user_id          BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    to_user_id            BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    to_phone              VARCHAR(16)   NOT NULL,
    from_code_hash        VARCHAR(255)  NOT NULL,
    to_code_hash          VARCHAR(255)  NOT NULL,
    from_verified_at      TIMESTAMPTZ   NULL,
    to_verified_at        TIMESTAMPTZ   NULL,
    attempts              INT           NOT NULL DEFAULT 0,
    expires_at            TIMESTAMPTZ   NOT NULL,
    status                VARCHAR(16)   NOT NULL DEFAULT 'pending',
    initiated_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    completed_at          TIMESTAMPTZ   NULL,
    cancelled_at          TIMESTAMPTZ   NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vehicle_transfers_uuid UNIQUE (uuid),
    CONSTRAINT chk_vehicle_transfers_to_phone CHECK (to_phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT chk_vehicle_transfers_to_user CHECK (to_user_id IS NULL OR to_user_id <> from_user_id),
    CONSTRAINT chk_vehicle_transfers_hashes CHECK (btrim(from_code_hash) <> '' AND btrim(to_code_hash) <> ''),
    CONSTRAINT chk_vehicle_transfers_attempts CHECK (attempts >= 0),
    CONSTRAINT chk_vehicle_transfers_expires CHECK (expires_at > created_at),
    CONSTRAINT chk_vehicle_transfers_status CHECK (status IN ('pending', 'completed', 'cancelled', 'expired')),
    CONSTRAINT chk_vehicle_transfers_completed CHECK (
        (status = 'completed') = (completed_at IS NOT NULL)
        AND (status <> 'completed' OR (to_user_id IS NOT NULL
             AND from_verified_at IS NOT NULL AND to_verified_at IS NOT NULL))
    ),
    CONSTRAINT chk_vehicle_transfers_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

-- One open transfer per vehicle.
CREATE UNIQUE INDEX uq_vehicle_transfers_pending ON vehicle_transfers (vehicle_id) WHERE status = 'pending';
CREATE INDEX idx_vehicle_transfers_vehicle ON vehicle_transfers (vehicle_id, created_at);
CREATE INDEX idx_vehicle_transfers_org_created ON vehicle_transfers (organization_id, created_at);
CREATE INDEX idx_vehicle_transfers_pending_expiry ON vehicle_transfers (expires_at) WHERE status = 'pending';

CREATE TRIGGER trg_vehicle_transfers_set_updated_at
    BEFORE UPDATE ON vehicle_transfers
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_vehicle_transfers_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON vehicle_transfers
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- The transfer starts from the vehicle's current owner; only a pending
-- transfer changes status.
CREATE FUNCTION vehicle_transfers_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF EXISTS (SELECT 1 FROM vehicles v
                   WHERE v.id = NEW.vehicle_id AND v.user_id IS DISTINCT FROM NEW.from_user_id) THEN
            RAISE EXCEPTION 'vehicle_transfers: vehicle % does not belong to user %', NEW.vehicle_id, NEW.from_user_id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.vehicle_id IS DISTINCT FROM OLD.vehicle_id OR NEW.from_user_id IS DISTINCT FROM OLD.from_user_id THEN
        RAISE EXCEPTION 'vehicle_transfers: vehicle and current owner are immutable (transfer %)', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'vehicle_transfers: status % is final (transfer %)', OLD.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_vehicle_transfers_check_row
    BEFORE INSERT OR UPDATE ON vehicle_transfers
    FOR EACH ROW
    EXECUTE FUNCTION vehicle_transfers_check_row();

-- 4. Google Business review link of the organization (decision 7). The
-- review request is sent only when it is set.
ALTER TABLE organizations
    ADD COLUMN google_business_url TEXT NULL,
    ADD CONSTRAINT chk_organizations_google_business_url CHECK (
        google_business_url IS NULL
        OR (google_business_url ~ '^https://[^[:space:]/?#]+([/?#][^[:space:]]*)?$'
            AND char_length(google_business_url) <= 2048)
    );

-- 5. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (TestMigrationMatchesCatalog compares the two). warranties.void is
-- center-only (scopes brand/all).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read warranties', 'warranties.read', 'warranties',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all', 'customer']::text[], false, false,
       'Warranties opened from completed services: period, status, covered product.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Void warranties', 'warranties.void', 'warranties',
       ARRAY['brand', 'all']::text[], false, false,
       'Void a warranty (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Transfer vehicles', 'vehicles.transfer', 'vehicles',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Transfer a vehicle and its active warranties to a new owner with two codes.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'warranties.read', 'all'),
    ('super_admin', 'warranties.void', 'all'),
    ('super_admin', 'vehicles.transfer', 'all'),
    ('center_staff', 'warranties.read', 'brand'),
    ('center_staff', 'warranties.void', 'brand'),
    ('center_staff', 'vehicles.transfer', 'brand'),
    ('distributor_owner', 'warranties.read', 'subtree'),
    ('distributor_owner', 'vehicles.transfer', 'subtree'),
    ('distributor_staff', 'warranties.read', 'subtree'),
    ('distributor_staff', 'vehicles.transfer', 'subtree'),
    ('dealer_owner', 'warranties.read', 'managed'),
    ('dealer_owner', 'vehicles.transfer', 'managed'),
    ('dealer_staff', 'warranties.read', 'managed'),
    ('dealer_staff', 'vehicles.transfer', 'own'),
    ('customer', 'warranties.read', 'customer'),
    ('fleet', 'warranties.read', 'customer')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
