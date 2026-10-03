-- TEC-305 (F3-08a): non-product service catalog, distributor price
-- overrides, subscriptions with idempotent period accounting, early
-- cancellation requests and the "module bundle" item type.
--
--   * service_catalog_items: defined by the center of a brand only
--     (organization_id = the brand's center, enforced by a trigger). The
--     category list is fixed in code. recurrence is one_time, monthly or
--     yearly; cancellation_fee is a static early-cancellation fee.
--     contract_template_id is optional and has NO foreign key yet: the
--     contract template table (F3-01a, TEC-285) is not merged; F3-08e adds
--     the FK.
--   * service_catalog_modules: modules a module_bundle item turns on. The
--     composite FK (item_id, item_category) -> items (id, category) with
--     item_category pinned to 'module_bundle' rejects any other category and
--     blocks changing the category of a bundle that still has modules.
--   * service_price_overrides: per-distributor price of an item; one row
--     per (item, distributor).
--   * service_subscriptions: an item assigned to a distributor or dealer.
--     The seller is always the brand's center (conservative default: a
--     distributor assigns without a margin, so the center sells).
--     assigned_by_org_id is the center or the distributor that assigned it.
--     ends_on is mandatory. Price, currency, recurrence and rate_snapshot are
--     frozen at assignment. contract_id stays without FK (F3-08e).
--   * service_subscription_periods: one row per accounted period;
--     UNIQUE (subscription_id, period_start) makes period posting
--     idempotent.
--   * service_subscription_cancel_requests: early cancellation requested by
--     the dealer or distributor, decided by the center; the cancellation fee
--     is frozen on the request. One pending request per subscription.
--   * Permissions appended at the end of the catalog (sort_order MAX+10).
--   * No stock movement: services only produce accounting entries.

-- 1. Catalog items.
CREATE TABLE service_catalog_items (
    id                    BIGSERIAL      PRIMARY KEY,
    uuid                  UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name                  VARCHAR(200)   NOT NULL,
    description           TEXT           NOT NULL DEFAULT '',
    category              VARCHAR(16)    NOT NULL,
    default_price         NUMERIC(18,2)  NOT NULL,
    currency              CHAR(3)        NOT NULL,
    recurrence            VARCHAR(16)    NOT NULL,
    cancellation_fee      NUMERIC(18,2)  NOT NULL DEFAULT 0,
    -- FK to the contract template table is added by F3-08e (TEC-285 first).
    contract_template_id  BIGINT         NULL,
    is_active             BOOLEAN        NOT NULL DEFAULT true,
    created_at            TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_catalog_items_uuid UNIQUE (uuid),
    -- Targets of the module-bundle and subscription composite FKs.
    CONSTRAINT uq_service_catalog_items_id_category UNIQUE (id, category),
    CONSTRAINT uq_service_catalog_items_id_brand UNIQUE (id, brand_id),
    CONSTRAINT chk_service_catalog_items_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_service_catalog_items_description CHECK (char_length(description) <= 10000),
    CONSTRAINT chk_service_catalog_items_category CHECK (
        category IN ('advertising', 'training', 'setup', 'software', 'module_bundle', 'other')),
    CONSTRAINT chk_service_catalog_items_recurrence CHECK (recurrence IN ('one_time', 'monthly', 'yearly')),
    CONSTRAINT chk_service_catalog_items_price CHECK (default_price >= 0),
    CONSTRAINT chk_service_catalog_items_fee CHECK (cancellation_fee >= 0),
    CONSTRAINT chk_service_catalog_items_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX idx_service_catalog_items_brand ON service_catalog_items (brand_id, is_active, name);
CREATE INDEX idx_service_catalog_items_org ON service_catalog_items (organization_id);

CREATE TRIGGER trg_service_catalog_items_set_updated_at
    BEFORE UPDATE ON service_catalog_items
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The owner is the center of the item's brand; owner and brand never change.
CREATE FUNCTION service_catalog_items_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type  TEXT;
    o_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id THEN
            RAISE EXCEPTION 'service_catalog_items: owner of item % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (o_type IS DISTINCT FROM 'center' OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'service_catalog_items: owner % must be the center organization of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_catalog_items_check_row
    BEFORE INSERT OR UPDATE ON service_catalog_items
    FOR EACH ROW
    EXECUTE FUNCTION service_catalog_items_check_row();

-- 2. Modules of a module_bundle item.
CREATE TABLE service_catalog_modules (
    item_id        BIGINT       NOT NULL,
    item_category  VARCHAR(16)  NOT NULL DEFAULT 'module_bundle',
    module_key     VARCHAR(64)  NOT NULL REFERENCES modules (key) ON DELETE RESTRICT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_service_catalog_modules PRIMARY KEY (item_id, module_key),
    CONSTRAINT chk_service_catalog_modules_bundle CHECK (item_category = 'module_bundle'),
    CONSTRAINT fk_service_catalog_modules_item FOREIGN KEY (item_id, item_category)
        REFERENCES service_catalog_items (id, category) ON DELETE CASCADE
);

CREATE INDEX idx_service_catalog_modules_module ON service_catalog_modules (module_key);

-- 3. Per-distributor price overrides.
CREATE TABLE service_price_overrides (
    id               BIGSERIAL      PRIMARY KEY,
    item_id          BIGINT         NOT NULL,
    organization_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id         BIGINT         NOT NULL,
    price            NUMERIC(18,2)  NOT NULL,
    currency         CHAR(3)        NOT NULL,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_price_overrides_item_org UNIQUE (item_id, organization_id),
    CONSTRAINT fk_service_price_overrides_item FOREIGN KEY (item_id, brand_id)
        REFERENCES service_catalog_items (id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_service_price_overrides_price CHECK (price >= 0),
    CONSTRAINT chk_service_price_overrides_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX idx_service_price_overrides_org ON service_price_overrides (organization_id);

CREATE TRIGGER trg_service_price_overrides_set_updated_at
    BEFORE UPDATE ON service_price_overrides
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The override belongs to a distributor of the item's brand.
CREATE FUNCTION service_price_overrides_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type  TEXT;
    o_brand BIGINT;
BEGIN
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (o_type IS DISTINCT FROM 'distributor' OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'service_price_overrides: organization % must be a distributor of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_price_overrides_check_row
    BEFORE INSERT OR UPDATE ON service_price_overrides
    FOR EACH ROW
    EXECUTE FUNCTION service_price_overrides_check_row();

-- 4. Subscriptions. organization_id is the receiving organization.
CREATE TABLE service_subscriptions (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id             BIGINT         NOT NULL,
    seller_org_id        BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    item_id              BIGINT         NOT NULL,
    assigned_by_org_id   BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    assigned_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    starts_on            DATE           NOT NULL,
    ends_on              DATE           NOT NULL,
    recurrence           VARCHAR(16)    NOT NULL,
    price                NUMERIC(18,2)  NOT NULL,
    currency             CHAR(3)        NOT NULL,
    rate_snapshot        JSONB          NOT NULL DEFAULT '{}'::jsonb,
    cancellation_fee     NUMERIC(18,2)  NOT NULL DEFAULT 0,
    status               VARCHAR(16)    NOT NULL DEFAULT 'active',
    -- FK to the contracts table is added by F3-08e (TEC-285 first).
    contract_id          BIGINT         NULL,
    cancelled_at         TIMESTAMPTZ    NULL,
    expired_at           TIMESTAMPTZ    NULL,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_subscriptions_uuid UNIQUE (uuid),
    -- Target of the period and cancel-request composite FKs.
    CONSTRAINT uq_service_subscriptions_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_service_subscriptions_item FOREIGN KEY (item_id, brand_id)
        REFERENCES service_catalog_items (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_subscriptions_period CHECK (ends_on >= starts_on),
    CONSTRAINT chk_service_subscriptions_recurrence CHECK (recurrence IN ('one_time', 'monthly', 'yearly')),
    CONSTRAINT chk_service_subscriptions_price CHECK (price >= 0),
    CONSTRAINT chk_service_subscriptions_fee CHECK (cancellation_fee >= 0),
    CONSTRAINT chk_service_subscriptions_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_service_subscriptions_rate_snapshot CHECK (jsonb_typeof(rate_snapshot) = 'object'),
    CONSTRAINT chk_service_subscriptions_status CHECK (
        status IN ('active', 'cancel_requested', 'cancelled', 'expired')),
    CONSTRAINT chk_service_subscriptions_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL)),
    CONSTRAINT chk_service_subscriptions_expired CHECK ((status = 'expired') = (expired_at IS NOT NULL))
);

CREATE INDEX idx_service_subscriptions_org ON service_subscriptions (organization_id, status, ends_on);
CREATE INDEX idx_service_subscriptions_brand ON service_subscriptions (brand_id, status, ends_on);
CREATE INDEX idx_service_subscriptions_item ON service_subscriptions (item_id);
CREATE INDEX idx_service_subscriptions_assigned_by ON service_subscriptions (assigned_by_org_id, created_at);
-- Expiry and period cron.
CREATE INDEX idx_service_subscriptions_open_end ON service_subscriptions (ends_on)
    WHERE status IN ('active', 'cancel_requested');

CREATE TRIGGER trg_service_subscriptions_set_updated_at
    BEFORE UPDATE ON service_subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The receiver is a distributor or dealer of the brand, the seller is the
-- brand's center and the assigner is the center or a distributor of the
-- brand (the subtree rule lives in the use case). Identity, period start,
-- price snapshot and the item never change; cancelled and expired are
-- final.
CREATE FUNCTION service_subscriptions_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type  TEXT;
    o_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id
           OR NEW.seller_org_id <> OLD.seller_org_id
           OR NEW.item_id <> OLD.item_id
           OR NEW.assigned_by_org_id <> OLD.assigned_by_org_id
           OR NEW.assigned_by_user_id IS DISTINCT FROM OLD.assigned_by_user_id
           OR NEW.starts_on <> OLD.starts_on
           OR NEW.recurrence <> OLD.recurrence
           OR NEW.price <> OLD.price
           OR NEW.currency <> OLD.currency
           OR NEW.rate_snapshot <> OLD.rate_snapshot
           OR NEW.cancellation_fee <> OLD.cancellation_fee THEN
            RAISE EXCEPTION 'service_subscriptions: identity and price snapshot of subscription % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        IF NEW.status IS DISTINCT FROM OLD.status AND OLD.status IN ('cancelled', 'expired') THEN
            RAISE EXCEPTION 'service_subscriptions: status % is final (subscription %)', OLD.status, OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;

    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (o_type NOT IN ('distributor', 'dealer') OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'service_subscriptions: receiver % must be a distributor or dealer of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.seller_org_id;
    IF FOUND AND (o_type IS DISTINCT FROM 'center' OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'service_subscriptions: seller % must be the center organization of brand %',
            NEW.seller_org_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.assigned_by_org_id;
    IF FOUND AND (o_type NOT IN ('center', 'distributor') OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'service_subscriptions: assigner % must be the center or a distributor of brand %',
            NEW.assigned_by_org_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_subscriptions_check_row
    BEFORE INSERT OR UPDATE ON service_subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION service_subscriptions_check_row();

-- 5. Accounted periods (idempotency key of period posting).
CREATE TABLE service_subscription_periods (
    id               BIGSERIAL    PRIMARY KEY,
    subscription_id  BIGINT       NOT NULL,
    organization_id  BIGINT       NOT NULL,
    brand_id         BIGINT       NOT NULL,
    period_start     DATE         NOT NULL,
    period_end       DATE         NOT NULL,
    posted_at        TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_subscription_periods_start UNIQUE (subscription_id, period_start),
    CONSTRAINT fk_service_subscription_periods_subscription
        FOREIGN KEY (subscription_id, organization_id, brand_id)
        REFERENCES service_subscriptions (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_subscription_periods_range CHECK (period_end >= period_start)
);

CREATE INDEX idx_service_subscription_periods_unposted ON service_subscription_periods (period_start)
    WHERE posted_at IS NULL;

-- 6. Early cancellation requests.
CREATE TABLE service_subscription_cancel_requests (
    id                      BIGSERIAL      PRIMARY KEY,
    uuid                    UUID           NOT NULL DEFAULT gen_random_uuid(),
    subscription_id         BIGINT         NOT NULL,
    organization_id         BIGINT         NOT NULL,
    brand_id                BIGINT         NOT NULL,
    requested_by_user_id    BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    requested_by_org_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    reason                  TEXT           NOT NULL,
    status                  VARCHAR(16)    NOT NULL DEFAULT 'pending',
    cancellation_fee        NUMERIC(18,2)  NOT NULL DEFAULT 0,
    currency                CHAR(3)        NOT NULL,
    decided_by_user_id      BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at              TIMESTAMPTZ    NULL,
    decision_note           TEXT           NULL,
    created_at              TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_subscription_cancel_requests_uuid UNIQUE (uuid),
    CONSTRAINT fk_service_subscription_cancel_requests_subscription
        FOREIGN KEY (subscription_id, organization_id, brand_id)
        REFERENCES service_subscriptions (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_subscription_cancel_requests_reason CHECK (
        btrim(reason) <> '' AND char_length(reason) <= 5000),
    CONSTRAINT chk_service_subscription_cancel_requests_status CHECK (
        status IN ('pending', 'approved', 'rejected')),
    CONSTRAINT chk_service_subscription_cancel_requests_fee CHECK (cancellation_fee >= 0),
    CONSTRAINT chk_service_subscription_cancel_requests_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_service_subscription_cancel_requests_decided CHECK (
        (status = 'pending') = (decided_at IS NULL))
);

-- One open request per subscription.
CREATE UNIQUE INDEX uq_service_subscription_cancel_requests_pending
    ON service_subscription_cancel_requests (subscription_id) WHERE status = 'pending';
CREATE INDEX idx_service_subscription_cancel_requests_brand
    ON service_subscription_cancel_requests (brand_id, status, created_at);
CREATE INDEX idx_service_subscription_cancel_requests_org
    ON service_subscription_cancel_requests (organization_id, created_at);

CREATE TRIGGER trg_service_subscription_cancel_requests_set_updated_at
    BEFORE UPDATE ON service_subscription_cancel_requests
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- A decision is final; the request, its subscription and the fee snapshot
-- never change.
CREATE FUNCTION service_subscription_cancel_requests_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.uuid <> OLD.uuid
       OR NEW.subscription_id <> OLD.subscription_id
       OR NEW.organization_id <> OLD.organization_id
       OR NEW.brand_id <> OLD.brand_id
       OR NEW.requested_by_org_id <> OLD.requested_by_org_id
       OR NEW.requested_by_user_id IS DISTINCT FROM OLD.requested_by_user_id
       OR NEW.reason <> OLD.reason
       OR NEW.cancellation_fee <> OLD.cancellation_fee
       OR NEW.currency <> OLD.currency THEN
        RAISE EXCEPTION 'service_subscription_cancel_requests: request % cannot change', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF OLD.status <> 'pending' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'service_subscription_cancel_requests: request % is already %', OLD.id, OLD.status
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_subscription_cancel_requests_check_row
    BEFORE UPDATE ON service_subscription_cancel_requests
    FOR EACH ROW
    EXECUTE FUNCTION service_subscription_cancel_requests_check_row();

-- 7. Permissions (appended last, sort_order MAX+10 each; idempotent).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage service catalog', 'service_catalog.manage', 'service_catalog',
       ARRAY['brand', 'all']::text[], false, false,
       'Define non-product services, module bundles and distributor prices (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read service catalog', 'service_catalog.read', 'service_catalog',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Non-product services of the brand with their prices.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Assign service subscriptions', 'service_subscriptions.assign', 'service_subscriptions',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Assign a catalog service to a distributor or dealer (distributors without a margin).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read service subscriptions', 'service_subscriptions.read', 'service_subscriptions',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Service subscriptions, their periods and cancellation requests.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Request subscription cancellation', 'service_subscriptions.cancel_request', 'service_subscriptions',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Request early cancellation of a service subscription of the organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Approve subscription cancellation', 'service_subscriptions.cancel_approve', 'service_subscriptions',
       ARRAY['brand', 'all']::text[], false, false,
       'Approve or reject early cancellation requests; the static cancellation fee applies (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'service_catalog.manage', 'all'),
    ('super_admin', 'service_catalog.read', 'all'),
    ('super_admin', 'service_subscriptions.assign', 'all'),
    ('super_admin', 'service_subscriptions.read', 'all'),
    ('super_admin', 'service_subscriptions.cancel_request', 'all'),
    ('super_admin', 'service_subscriptions.cancel_approve', 'all'),
    ('center_staff', 'service_catalog.manage', 'brand'),
    ('center_staff', 'service_catalog.read', 'brand'),
    ('center_staff', 'service_subscriptions.assign', 'brand'),
    ('center_staff', 'service_subscriptions.read', 'brand'),
    ('center_staff', 'service_subscriptions.cancel_approve', 'brand'),
    ('center_accounting', 'service_catalog.manage', 'brand'),
    ('center_accounting', 'service_catalog.read', 'brand'),
    ('center_accounting', 'service_subscriptions.assign', 'brand'),
    ('center_accounting', 'service_subscriptions.read', 'brand'),
    ('center_accounting', 'service_subscriptions.cancel_approve', 'brand'),
    ('distributor_owner', 'service_catalog.read', 'managed'),
    ('distributor_owner', 'service_subscriptions.assign', 'subtree'),
    ('distributor_owner', 'service_subscriptions.read', 'subtree'),
    ('distributor_owner', 'service_subscriptions.cancel_request', 'managed'),
    ('distributor_accounting', 'service_subscriptions.read', 'managed'),
    ('dealer_owner', 'service_subscriptions.read', 'managed'),
    ('dealer_owner', 'service_subscriptions.cancel_request', 'managed'),
    ('dealer_accounting', 'service_subscriptions.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
