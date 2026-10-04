-- TEC-334 (F3-06a): warranty claim schema, permissions and sqlc.
-- Endpoints, AI triage and notifications land in TEC-335+; this migration
-- only fixes the data model and database guards.
--
--   * warranty_claims: a dealer or distributor opens a claim against a
--     warranty. organization_id is the opening organization, brand_id its
--     brand (= the warranty brand). claim_no is sequential per
--     organization. Ownership (decided here): the opening organization is
--     either the organization that performed the warranted service
--     (warranty.organization_id) or a distributor ancestor of it (subtree).
--     Sibling dealers, other distributors' trees and other brands are
--     rejected by trigger. service_id, vehicle_id and customer_user_id are
--     copied from the warranty (customer = current warranty holder) at
--     insert time and never change.
--   * One live claim per warranty: a partial unique index over the statuses
--     other than rejected and closed.
--   * coverage_check is the frozen JSON result of the coverage check; the
--     ai_* columns are filled by the AI triage later and stay NULL here.
--   * reapply_service_id links the re-application service; the reverse link
--     is services.warranty_claim_id. Both must stay in the claim's brand and
--     vehicle.
--   * warranty_claim_events is append-only. Creation and every status change
--     are written by trigger (actor = updated_by_user_id / created_by);
--     notes and other events are added by the application.

-- 1. Claims ----------------------------------------------------------------
CREATE TABLE warranty_claims (
    id                   BIGSERIAL     PRIMARY KEY,
    uuid                 UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id             BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    claim_no             BIGINT        NOT NULL,
    warranty_id          BIGINT        NOT NULL REFERENCES warranties (id) ON DELETE RESTRICT,
    service_id           BIGINT        NOT NULL REFERENCES services (id) ON DELETE RESTRICT,
    vehicle_id           BIGINT        NOT NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    customer_user_id     BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    description          TEXT          NOT NULL,
    status               VARCHAR(16)   NOT NULL DEFAULT 'open',
    rejection_reason     TEXT          NULL,
    coverage_check       JSONB         NOT NULL DEFAULT '{}'::jsonb,
    -- AI triage (filled later; NULL until then).
    ai_damage_type       VARCHAR(64)   NULL,
    ai_summary           TEXT          NULL,
    ai_confidence        NUMERIC(4,3)  NULL,
    ai_triaged_at        TIMESTAMPTZ   NULL,
    reapply_service_id   BIGINT        NULL REFERENCES services (id) ON DELETE RESTRICT,
    decided_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at           TIMESTAMPTZ   NULL,
    created_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    updated_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    closed_at            TIMESTAMPTZ   NULL,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warranty_claims_uuid UNIQUE (uuid),
    CONSTRAINT uq_warranty_claims_org_no UNIQUE (organization_id, claim_no),
    CONSTRAINT uq_warranty_claims_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_warranty_claims_claim_no CHECK (claim_no > 0),
    CONSTRAINT chk_warranty_claims_description CHECK (
        btrim(description) <> '' AND char_length(description) <= 20000),
    CONSTRAINT chk_warranty_claims_status CHECK (status IN (
        'open', 'dealer_review', 'center_review', 'approved', 'rejected', 'reapplied', 'closed')),
    CONSTRAINT chk_warranty_claims_rejection CHECK (
        status <> 'rejected' OR (rejection_reason IS NOT NULL AND btrim(rejection_reason) <> '')),
    CONSTRAINT chk_warranty_claims_rejection_len CHECK (
        rejection_reason IS NULL OR char_length(rejection_reason) <= 5000),
    CONSTRAINT chk_warranty_claims_decided CHECK (
        status NOT IN ('approved', 'rejected', 'reapplied') OR decided_at IS NOT NULL),
    CONSTRAINT chk_warranty_claims_reapplied CHECK (
        status <> 'reapplied' OR reapply_service_id IS NOT NULL),
    CONSTRAINT chk_warranty_claims_closed CHECK ((status = 'closed') = (closed_at IS NOT NULL)),
    CONSTRAINT chk_warranty_claims_coverage CHECK (jsonb_typeof(coverage_check) = 'object'),
    CONSTRAINT chk_warranty_claims_ai_confidence CHECK (
        ai_confidence IS NULL OR (ai_confidence >= 0 AND ai_confidence <= 1)),
    CONSTRAINT chk_warranty_claims_ai_damage_type CHECK (
        ai_damage_type IS NULL OR btrim(ai_damage_type) <> '')
);

-- One live claim per warranty; a rejected or closed claim allows a new one.
CREATE UNIQUE INDEX uq_warranty_claims_live_warranty ON warranty_claims (warranty_id)
    WHERE status NOT IN ('rejected', 'closed');

CREATE INDEX idx_warranty_claims_org_status ON warranty_claims (organization_id, status, created_at DESC);
CREATE INDEX idx_warranty_claims_brand_status ON warranty_claims (brand_id, status, created_at DESC);
CREATE INDEX idx_warranty_claims_warranty ON warranty_claims (warranty_id, created_at DESC);
CREATE INDEX idx_warranty_claims_service ON warranty_claims (service_id);
CREATE INDEX idx_warranty_claims_vehicle ON warranty_claims (vehicle_id);
CREATE INDEX idx_warranty_claims_customer ON warranty_claims (customer_user_id);
CREATE INDEX idx_warranty_claims_reapply_service ON warranty_claims (reapply_service_id)
    WHERE reapply_service_id IS NOT NULL;

CREATE TRIGGER trg_warranty_claims_set_updated_at
    BEFORE UPDATE ON warranty_claims
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_warranty_claims_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON warranty_claims
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Ownership and copied references (see header), immutability of the owner
-- and the claimed warranty, closed is final, and the re-application service
-- stays in the claim's brand and vehicle.
CREATE FUNCTION warranty_claims_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    w_org      BIGINT;
    w_brand    BIGINT;
    w_service  BIGINT;
    w_vehicle  BIGINT;
    w_holder   BIGINT;
    c_type     VARCHAR(16);
    s_brand    BIGINT;
    s_vehicle  BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT organization_id, brand_id, service_id, vehicle_id, holder_user_id
          INTO w_org, w_brand, w_service, w_vehicle, w_holder
          FROM warranties WHERE id = NEW.warranty_id;
        IF FOUND THEN
            IF w_brand IS DISTINCT FROM NEW.brand_id THEN
                RAISE EXCEPTION 'warranty_claims: warranty % is outside brand %', NEW.warranty_id, NEW.brand_id
                    USING ERRCODE = 'check_violation';
            END IF;
            IF NEW.organization_id IS DISTINCT FROM w_org THEN
                SELECT type INTO c_type FROM organizations WHERE id = NEW.organization_id;
                IF c_type IS DISTINCT FROM 'distributor' OR NOT EXISTS (
                    WITH RECURSIVE ancestors(id, parent_id, depth) AS (
                        SELECT o.id, o.parent_id, 0 FROM organizations o WHERE o.id = w_org
                        UNION ALL
                        SELECT o.id, o.parent_id, a.depth + 1
                        FROM organizations o JOIN ancestors a ON o.id = a.parent_id
                        WHERE a.depth < 32
                    )
                    SELECT 1 FROM ancestors WHERE id = NEW.organization_id
                ) THEN
                    RAISE EXCEPTION 'warranty_claims: organization % may not claim warranty % of organization %',
                        NEW.organization_id, NEW.warranty_id, w_org
                        USING ERRCODE = 'check_violation';
                END IF;
            END IF;
            IF NEW.service_id IS DISTINCT FROM w_service
               OR NEW.vehicle_id IS DISTINCT FROM w_vehicle
               OR NEW.customer_user_id IS DISTINCT FROM w_holder THEN
                RAISE EXCEPTION 'warranty_claims: service, vehicle and customer must match warranty %', NEW.warranty_id
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
    ELSE
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id
           OR NEW.claim_no <> OLD.claim_no
           OR NEW.warranty_id <> OLD.warranty_id
           OR NEW.service_id <> OLD.service_id
           OR NEW.vehicle_id <> OLD.vehicle_id
           OR NEW.customer_user_id <> OLD.customer_user_id THEN
            RAISE EXCEPTION 'warranty_claims: owner and claimed warranty of claim % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        IF OLD.status = 'closed' AND NEW.status IS DISTINCT FROM OLD.status THEN
            RAISE EXCEPTION 'warranty_claims: claim % is closed', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF NEW.reapply_service_id IS NOT NULL
       AND (TG_OP = 'INSERT' OR NEW.reapply_service_id IS DISTINCT FROM OLD.reapply_service_id) THEN
        SELECT brand_id, vehicle_id INTO s_brand, s_vehicle FROM services WHERE id = NEW.reapply_service_id;
        IF FOUND AND (s_brand IS DISTINCT FROM NEW.brand_id OR s_vehicle IS DISTINCT FROM NEW.vehicle_id) THEN
            RAISE EXCEPTION 'warranty_claims: re-application service % is outside the brand or vehicle of claim %',
                NEW.reapply_service_id, NEW.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_warranty_claims_check_row
    BEFORE INSERT OR UPDATE ON warranty_claims
    FOR EACH ROW
    EXECUTE FUNCTION warranty_claims_check_row();

-- 2. Claim parts -----------------------------------------------------------
-- part_key is a key of the category's available_parts. The optional links
-- point at the covered service item, product and unit (lot/roll).
CREATE TABLE warranty_claim_parts (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    claim_id          BIGINT       NOT NULL,
    organization_id   BIGINT       NOT NULL,
    brand_id          BIGINT       NOT NULL,
    part_key          VARCHAR(64)  NOT NULL,
    service_item_id   BIGINT       NULL REFERENCES service_items (id) ON DELETE RESTRICT,
    product_id        BIGINT       NULL,
    unit_id           BIGINT       NULL,
    note              TEXT         NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warranty_claim_parts_uuid UNIQUE (uuid),
    CONSTRAINT uq_warranty_claim_parts_claim_part UNIQUE (claim_id, part_key),
    CONSTRAINT fk_warranty_claim_parts_claim FOREIGN KEY (claim_id, organization_id, brand_id)
        REFERENCES warranty_claims (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_warranty_claim_parts_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_warranty_claim_parts_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_warranty_claim_parts_part_key CHECK (part_key ~ '^[a-z0-9_]{1,64}$'),
    CONSTRAINT chk_warranty_claim_parts_note CHECK (char_length(note) <= 2000)
);

CREATE INDEX idx_warranty_claim_parts_claim ON warranty_claim_parts (claim_id, id);
CREATE INDEX idx_warranty_claim_parts_service_item ON warranty_claim_parts (service_item_id)
    WHERE service_item_id IS NOT NULL;
CREATE INDEX idx_warranty_claim_parts_unit ON warranty_claim_parts (unit_id) WHERE unit_id IS NOT NULL;

-- The linked service item belongs to the claimed service.
CREATE FUNCTION warranty_claim_parts_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.service_item_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM service_items si JOIN warranty_claims c ON c.service_id = si.service_id
        WHERE si.id = NEW.service_item_id AND c.id = NEW.claim_id
    ) THEN
        RAISE EXCEPTION 'warranty_claim_parts: service item % is not part of the claimed service', NEW.service_item_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_warranty_claim_parts_check_row
    BEFORE INSERT OR UPDATE ON warranty_claim_parts
    FOR EACH ROW
    EXECUTE FUNCTION warranty_claim_parts_check_row();

-- 3. Claim photos ----------------------------------------------------------
CREATE TABLE warranty_claim_photos (
    id                   BIGSERIAL     PRIMARY KEY,
    uuid                 UUID          NOT NULL DEFAULT gen_random_uuid(),
    claim_id             BIGINT        NOT NULL,
    organization_id      BIGINT        NOT NULL,
    brand_id             BIGINT        NOT NULL,
    storage_key          TEXT          NOT NULL,
    mime_type            VARCHAR(100)  NOT NULL,
    size_bytes           BIGINT        NOT NULL,
    sha256               CHAR(64)      NOT NULL,
    uploaded_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warranty_claim_photos_uuid UNIQUE (uuid),
    CONSTRAINT uq_warranty_claim_photos_storage_key UNIQUE (storage_key),
    CONSTRAINT uq_warranty_claim_photos_claim_sha UNIQUE (claim_id, sha256),
    CONSTRAINT fk_warranty_claim_photos_claim FOREIGN KEY (claim_id, organization_id, brand_id)
        REFERENCES warranty_claims (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_warranty_claim_photos_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_warranty_claim_photos_mime CHECK (mime_type ~ '^image/[a-z0-9.+-]+$'),
    CONSTRAINT chk_warranty_claim_photos_size CHECK (size_bytes > 0),
    CONSTRAINT chk_warranty_claim_photos_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_warranty_claim_photos_claim ON warranty_claim_photos (claim_id, created_at, id);

-- 4. Claim events (append-only) -------------------------------------------
CREATE TABLE warranty_claim_events (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    claim_id         BIGINT       NOT NULL,
    organization_id  BIGINT       NOT NULL,
    brand_id         BIGINT       NOT NULL,
    event_type       VARCHAR(32)  NOT NULL,
    from_status      VARCHAR(16)  NULL,
    to_status        VARCHAR(16)  NULL,
    note             TEXT         NULL,
    payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    actor_user_id    BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warranty_claim_events_uuid UNIQUE (uuid),
    CONSTRAINT fk_warranty_claim_events_claim FOREIGN KEY (claim_id, organization_id, brand_id)
        REFERENCES warranty_claims (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_warranty_claim_events_type CHECK (event_type IN (
        'created', 'status_changed', 'note', 'part_added', 'photo_added',
        'ai_triaged', 'reapply_linked')),
    CONSTRAINT chk_warranty_claim_events_status_change CHECK (
        (event_type = 'created' AND from_status IS NULL AND to_status IS NOT NULL)
        OR (event_type = 'status_changed' AND from_status IS NOT NULL AND to_status IS NOT NULL)
        OR (event_type NOT IN ('created', 'status_changed') AND from_status IS NULL AND to_status IS NULL)),
    CONSTRAINT chk_warranty_claim_events_note CHECK (
        (event_type <> 'note' OR (note IS NOT NULL AND btrim(note) <> ''))
        AND (note IS NULL OR char_length(note) <= 20000)),
    CONSTRAINT chk_warranty_claim_events_payload CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX idx_warranty_claim_events_claim ON warranty_claim_events (claim_id, created_at, id);
CREATE INDEX idx_warranty_claim_events_org ON warranty_claim_events (organization_id, created_at DESC);

CREATE FUNCTION warranty_claim_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'warranty_claim_events is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_warranty_claim_events_append_only
    BEFORE UPDATE OR DELETE ON warranty_claim_events
    FOR EACH ROW
    EXECUTE FUNCTION warranty_claim_events_append_only();

CREATE TRIGGER trg_warranty_claim_events_no_truncate
    BEFORE TRUNCATE ON warranty_claim_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION warranty_claim_events_append_only();

CREATE FUNCTION warranty_claims_write_event() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO warranty_claim_events
            (claim_id, organization_id, brand_id, event_type, to_status, payload, actor_user_id)
        VALUES (
            NEW.id, NEW.organization_id, NEW.brand_id, 'created', NEW.status,
            jsonb_build_object('warranty_id', NEW.warranty_id, 'claim_no', NEW.claim_no),
            NEW.created_by_user_id
        );
    ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO warranty_claim_events
            (claim_id, organization_id, brand_id, event_type, from_status, to_status, note, actor_user_id)
        VALUES (
            NEW.id, NEW.organization_id, NEW.brand_id, 'status_changed', OLD.status, NEW.status,
            CASE WHEN NEW.status = 'rejected' THEN NEW.rejection_reason END,
            NEW.updated_by_user_id
        );
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_warranty_claims_write_event
    AFTER INSERT OR UPDATE OF status ON warranty_claims
    FOR EACH ROW
    EXECUTE FUNCTION warranty_claims_write_event();

-- 5. Re-application service link -------------------------------------------
ALTER TABLE services
    ADD COLUMN warranty_claim_id BIGINT NULL REFERENCES warranty_claims (id) ON DELETE RESTRICT;

CREATE INDEX idx_services_warranty_claim ON services (warranty_claim_id) WHERE warranty_claim_id IS NOT NULL;

CREATE FUNCTION services_check_warranty_claim() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.warranty_claim_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM warranty_claims c
        WHERE c.id = NEW.warranty_claim_id
          AND c.brand_id = NEW.brand_id
          AND c.vehicle_id = NEW.vehicle_id
    ) THEN
        RAISE EXCEPTION 'services: warranty claim % is outside the brand or vehicle of service %',
            NEW.warranty_claim_id, NEW.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_services_check_warranty_claim
    BEFORE INSERT OR UPDATE OF warranty_claim_id, brand_id, vehicle_id ON services
    FOR EACH ROW
    EXECUTE FUNCTION services_check_warranty_claim();

-- 6. Permissions -----------------------------------------------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read warranty claims', 'warranty_claims.read', 'warranty_claims',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read warranty claims, their parts, photos and timeline.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write warranty claims', 'warranty_claims.write', 'warranty_claims',
       ARRAY['managed', 'all']::text[], false, false,
       'Open warranty claims of the managed organization and add parts, photos and notes.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Review warranty claims', 'warranty_claims.review', 'warranty_claims',
       ARRAY['subtree', 'all']::text[], false, false,
       'Review dealer warranty claims of the subtree and forward them to the center.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Decide warranty claims', 'warranty_claims.decide', 'warranty_claims',
       ARRAY['brand', 'all']::text[], false, false,
       'Approve or reject warranty claims of the brand.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'warranty_claims.read', 'all'),
    ('super_admin', 'warranty_claims.write', 'all'),
    ('super_admin', 'warranty_claims.review', 'all'),
    ('super_admin', 'warranty_claims.decide', 'all'),
    ('center_staff', 'warranty_claims.read', 'brand'),
    ('center_staff', 'warranty_claims.decide', 'brand'),
    ('distributor_owner', 'warranty_claims.read', 'subtree'),
    ('distributor_owner', 'warranty_claims.write', 'managed'),
    ('distributor_owner', 'warranty_claims.review', 'subtree'),
    ('distributor_staff', 'warranty_claims.read', 'subtree'),
    ('distributor_staff', 'warranty_claims.write', 'managed'),
    ('distributor_staff', 'warranty_claims.review', 'subtree'),
    ('dealer_owner', 'warranty_claims.read', 'managed'),
    ('dealer_owner', 'warranty_claims.write', 'managed'),
    ('dealer_staff', 'warranty_claims.read', 'managed'),
    ('dealer_staff', 'warranty_claims.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
