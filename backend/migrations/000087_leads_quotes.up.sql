-- TEC-312 (F3-03a): lead and quote schema, permissions and sqlc.
-- Endpoints and public forms land in TEC-313/314; this migration only fixes
-- the data model and database guards.

-- 1. Leads -----------------------------------------------------------------
CREATE TABLE leads (
    id                      BIGSERIAL     PRIMARY KEY,
    uuid                    UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id         BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    target_type             VARCHAR(32)   NOT NULL,
    customer_user_id        BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    vehicle_id              BIGINT        NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    candidate_company_name  VARCHAR(200)  NULL,
    candidate_contact_name  VARCHAR(200)  NULL,
    candidate_phone_e164    VARCHAR(16)   NULL,
    candidate_email         VARCHAR(255)  NULL,
    country_id              BIGINT        NULL REFERENCES countries (id) ON DELETE RESTRICT,
    province_id             BIGINT        NULL REFERENCES provinces (id) ON DELETE RESTRICT,
    district_id             BIGINT        NULL REFERENCES districts (id) ON DELETE RESTRICT,
    source                  VARCHAR(32)   NOT NULL,
    temperature             VARCHAR(16)   NOT NULL DEFAULT 'cold',
    status                  VARCHAR(16)   NOT NULL DEFAULT 'new',
    lost_reason             TEXT          NULL,
    follow_up_date          TIMESTAMPTZ   NULL,
    assignee_user_id        BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    notes                   TEXT          NOT NULL DEFAULT '',
    won_ref_type            VARCHAR(16)   NULL,
    won_ref_id              BIGINT        NULL,
    created_by_user_id      BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ   NULL,
    CONSTRAINT uq_leads_uuid UNIQUE (uuid),
    CONSTRAINT uq_leads_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_leads_target_type CHECK (
        target_type IN ('customer', 'dealer_candidate', 'distributor_candidate')),
    CONSTRAINT chk_leads_source CHECK (
        source IN ('incoming_call', 'outgoing_call', 'walk_in', 'whatsapp', 'social',
                   'referral', 'website', 'application_form', 'other')),
    CONSTRAINT chk_leads_temperature CHECK (temperature IN ('cold', 'warm', 'hot')),
    CONSTRAINT chk_leads_status CHECK (status IN ('new', 'contacted', 'quoted', 'won', 'lost')),
    CONSTRAINT chk_leads_lost_reason CHECK ((status = 'lost') = (lost_reason IS NOT NULL)),
    CONSTRAINT chk_leads_won_ref CHECK (
        (won_ref_type IS NULL AND won_ref_id IS NULL)
        OR (status = 'won' AND won_ref_type IN ('service', 'appointment', 'organization') AND won_ref_id IS NOT NULL)
    ),
    CONSTRAINT chk_leads_candidate_phone CHECK (
        candidate_phone_e164 IS NULL OR candidate_phone_e164 ~ '^\+[1-9][0-9]{7,14}$'),
    CONSTRAINT chk_leads_candidate_email CHECK (
        candidate_email IS NULL OR candidate_email ~* '^[^@\s]+@[^@\s]+\.[^@\s]+$'),
    CONSTRAINT chk_leads_candidate_company CHECK (
        candidate_company_name IS NULL OR btrim(candidate_company_name) <> ''),
    CONSTRAINT chk_leads_candidate_contact CHECK (
        candidate_contact_name IS NULL OR btrim(candidate_contact_name) <> ''),
    CONSTRAINT chk_leads_notes CHECK (char_length(notes) <= 20000)
);

CREATE INDEX idx_leads_org_status ON leads (organization_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_leads_brand_status ON leads (brand_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_leads_assignee ON leads (assignee_user_id, follow_up_date) WHERE deleted_at IS NULL AND assignee_user_id IS NOT NULL;
CREATE INDEX idx_leads_customer ON leads (customer_user_id) WHERE customer_user_id IS NOT NULL;
CREATE INDEX idx_leads_vehicle ON leads (vehicle_id) WHERE vehicle_id IS NOT NULL;
CREATE INDEX idx_leads_follow_up ON leads (follow_up_date) WHERE follow_up_date IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER trg_leads_set_updated_at
    BEFORE UPDATE ON leads
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_leads_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON leads
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE FUNCTION leads_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_brand BIGINT;
    v_user  BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id THEN
            RAISE EXCEPTION 'leads: owner of lead % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF NEW.vehicle_id IS NOT NULL THEN
        SELECT brand_id, user_id INTO v_brand, v_user FROM vehicles WHERE id = NEW.vehicle_id;
        IF FOUND THEN
            IF v_brand IS DISTINCT FROM NEW.brand_id THEN
                RAISE EXCEPTION 'leads: vehicle % is outside brand %', NEW.vehicle_id, NEW.brand_id
                    USING ERRCODE = 'check_violation';
            END IF;
            IF NEW.customer_user_id IS NOT NULL AND v_user IS DISTINCT FROM NEW.customer_user_id THEN
                RAISE EXCEPTION 'leads: vehicle % does not belong to customer %',
                    NEW.vehicle_id, NEW.customer_user_id
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_leads_check_row
    BEFORE INSERT OR UPDATE ON leads
    FOR EACH ROW
    EXECUTE FUNCTION leads_check_row();

-- 2. Lead events -----------------------------------------------------------
CREATE TABLE lead_events (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    lead_id          BIGINT       NOT NULL REFERENCES leads (id) ON DELETE RESTRICT,
    organization_id  BIGINT       NOT NULL,
    brand_id         BIGINT       NOT NULL,
    event_type       VARCHAR(32)  NOT NULL,
    payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    actor_user_id    BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_lead_events_uuid UNIQUE (uuid),
    CONSTRAINT fk_lead_events_lead_scope FOREIGN KEY (lead_id, organization_id, brand_id)
        REFERENCES leads (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_lead_events_type CHECK (
        event_type IN ('created', 'status_changed', 'note', 'call', 'message',
                       'quote_sent', 'assigned', 'follow_up_set', 'converted')),
    CONSTRAINT chk_lead_events_payload CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX idx_lead_events_lead ON lead_events (lead_id, created_at, id);
CREATE INDEX idx_lead_events_org ON lead_events (organization_id, created_at DESC);

CREATE FUNCTION lead_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'lead_events is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_lead_events_append_only
    BEFORE UPDATE OR DELETE ON lead_events
    FOR EACH ROW
    EXECUTE FUNCTION lead_events_append_only();

CREATE TRIGGER trg_lead_events_no_truncate
    BEFORE TRUNCATE ON lead_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION lead_events_append_only();

CREATE FUNCTION leads_write_event() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO lead_events (lead_id, organization_id, brand_id, event_type, payload, actor_user_id)
        VALUES (
            NEW.id, NEW.organization_id, NEW.brand_id, 'created',
            jsonb_build_object('status', NEW.status, 'target_type', NEW.target_type),
            NEW.created_by_user_id
        );
    ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO lead_events (lead_id, organization_id, brand_id, event_type, payload)
        VALUES (
            NEW.id, NEW.organization_id, NEW.brand_id, 'status_changed',
            jsonb_build_object('from_status', OLD.status, 'to_status', NEW.status)
        );
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_leads_write_event
    AFTER INSERT OR UPDATE OF status ON leads
    FOR EACH ROW
    EXECUTE FUNCTION leads_write_event();

-- 3. Quotes ----------------------------------------------------------------
CREATE TABLE quotes (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    lead_id             BIGINT         NOT NULL,
    quote_no            BIGINT         NOT NULL,
    currency            CHAR(3)        NOT NULL,
    subtotal            NUMERIC(18,2)  NOT NULL DEFAULT 0,
    discount_total      NUMERIC(18,2)  NOT NULL DEFAULT 0,
    tax_total           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    grand_total         NUMERIC(18,2)  NOT NULL DEFAULT 0,
    valid_until         DATE           NULL,
    status              VARCHAR(16)    NOT NULL DEFAULT 'draft',
    public_token        UUID           NOT NULL DEFAULT gen_random_uuid(),
    created_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    sent_at             TIMESTAMPTZ    NULL,
    accepted_at         TIMESTAMPTZ    NULL,
    rejected_at         TIMESTAMPTZ    NULL,
    expired_at          TIMESTAMPTZ    NULL,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ    NULL,
    CONSTRAINT uq_quotes_uuid UNIQUE (uuid),
    CONSTRAINT uq_quotes_public_token UNIQUE (public_token),
    CONSTRAINT uq_quotes_org_no UNIQUE (organization_id, quote_no),
    CONSTRAINT uq_quotes_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_quotes_lead FOREIGN KEY (lead_id, organization_id, brand_id)
        REFERENCES leads (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_quotes_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_quotes_amounts CHECK (
        subtotal >= 0 AND discount_total >= 0 AND tax_total >= 0 AND grand_total >= 0),
    CONSTRAINT chk_quotes_status CHECK (status IN ('draft', 'sent', 'accepted', 'rejected', 'expired')),
    CONSTRAINT chk_quotes_status_times CHECK (
        (status <> 'sent' OR sent_at IS NOT NULL)
        AND (status <> 'accepted' OR accepted_at IS NOT NULL)
        AND (status <> 'rejected' OR rejected_at IS NOT NULL)
        AND (status <> 'expired' OR expired_at IS NOT NULL)
    )
);

CREATE INDEX idx_quotes_lead ON quotes (lead_id, created_at DESC);
CREATE INDEX idx_quotes_org_status ON quotes (organization_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_quotes_valid_until ON quotes (valid_until) WHERE status IN ('draft', 'sent') AND valid_until IS NOT NULL;

CREATE TRIGGER trg_quotes_set_updated_at
    BEFORE UPDATE ON quotes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_quotes_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON quotes
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. Quote lines -----------------------------------------------------------
CREATE TABLE quote_lines (
    id                       BIGSERIAL      PRIMARY KEY,
    quote_id                 BIGINT         NOT NULL,
    organization_id          BIGINT         NOT NULL,
    brand_id                 BIGINT         NOT NULL,
    line_type                VARCHAR(16)    NOT NULL,
    product_id               BIGINT         NULL,
    service_catalog_item_id  BIGINT         NULL,
    description_snapshot     TEXT           NOT NULL,
    quantity                 NUMERIC(12,3)  NOT NULL DEFAULT 1,
    unit_price               NUMERIC(18,2)  NOT NULL DEFAULT 0,
    discount_amount          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    line_total               NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sort_order               INTEGER        NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_quote_lines_quote FOREIGN KEY (quote_id, organization_id, brand_id)
        REFERENCES quotes (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_quote_lines_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_quote_lines_service_item FOREIGN KEY (service_catalog_item_id, brand_id)
        REFERENCES service_catalog_items (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_quote_lines_type CHECK (line_type IN ('product', 'catalog_service')),
    CONSTRAINT chk_quote_lines_reference CHECK (
        (line_type = 'product' AND product_id IS NOT NULL AND service_catalog_item_id IS NULL)
        OR (line_type = 'catalog_service' AND service_catalog_item_id IS NOT NULL AND product_id IS NULL)
    ),
    CONSTRAINT chk_quote_lines_description CHECK (btrim(description_snapshot) <> ''),
    CONSTRAINT chk_quote_lines_amounts CHECK (
        quantity > 0 AND unit_price >= 0 AND discount_amount >= 0 AND line_total >= 0)
);

CREATE INDEX idx_quote_lines_quote ON quote_lines (quote_id, sort_order, id);
CREATE INDEX idx_quote_lines_product ON quote_lines (product_id) WHERE product_id IS NOT NULL;
CREATE INDEX idx_quote_lines_service_item ON quote_lines (service_catalog_item_id) WHERE service_catalog_item_id IS NOT NULL;

-- 5. Quote deliveries and reminders ---------------------------------------
CREATE TABLE quote_deliveries (
    id                BIGSERIAL     PRIMARY KEY,
    quote_id          BIGINT        NOT NULL,
    organization_id   BIGINT        NOT NULL,
    brand_id          BIGINT        NOT NULL,
    channel           VARCHAR(16)   NOT NULL,
    status            VARCHAR(16)   NOT NULL DEFAULT 'pending',
    provider_ref      VARCHAR(255)  NULL,
    error_message     TEXT          NULL,
    sent_at           TIMESTAMPTZ   NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_quote_deliveries_quote FOREIGN KEY (quote_id, organization_id, brand_id)
        REFERENCES quotes (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_quote_deliveries_channel CHECK (channel IN ('whatsapp')),
    CONSTRAINT chk_quote_deliveries_status CHECK (status IN ('pending', 'sent', 'delivered', 'read', 'failed'))
);

CREATE INDEX idx_quote_deliveries_quote ON quote_deliveries (quote_id, created_at DESC);
CREATE INDEX idx_quote_deliveries_provider ON quote_deliveries (provider_ref) WHERE provider_ref IS NOT NULL;

CREATE TRIGGER trg_quote_deliveries_set_updated_at
    BEFORE UPDATE ON quote_deliveries
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE quote_reminders (
    id                BIGSERIAL    PRIMARY KEY,
    quote_id          BIGINT       NOT NULL,
    organization_id   BIGINT       NOT NULL,
    brand_id          BIGINT       NOT NULL,
    scheduled_at      TIMESTAMPTZ  NOT NULL,
    sent_at           TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_quote_reminders_quote FOREIGN KEY (quote_id, organization_id, brand_id)
        REFERENCES quotes (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_quote_reminders_sent CHECK (sent_at IS NULL OR sent_at >= scheduled_at)
);

CREATE INDEX idx_quote_reminders_due ON quote_reminders (scheduled_at) WHERE sent_at IS NULL;
CREATE INDEX idx_quote_reminders_quote ON quote_reminders (quote_id, scheduled_at);

-- 6. Permissions -----------------------------------------------------------
UPDATE permissions
SET description = 'Read leads of the managed organization.',
    scopes = ARRAY['managed', 'subtree', 'brand', 'all']::text[]
WHERE slug = 'leads.read';

UPDATE permissions
SET description = 'Create and update leads of the managed organization.',
    scopes = ARRAY['managed', 'subtree', 'brand', 'all']::text[]
WHERE slug = 'leads.write';

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read quotes', 'quotes.read', 'quotes',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read quotes and quote deliveries of the managed organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write quotes', 'quotes.write', 'quotes',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Create, edit, send and decide quotes of the managed organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Convert lead to organization', 'leads.convert_org', 'leads',
       ARRAY['subtree', 'brand', 'all']::text[], false, false,
       'Convert dealer candidates in the distributor subtree or center brand; distributor candidates are center-only.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'quotes.read', 'all'),
    ('super_admin', 'quotes.write', 'all'),
    ('super_admin', 'leads.convert_org', 'all'),
    ('center_staff', 'leads.read', 'managed'),
    ('center_staff', 'leads.write', 'managed'),
    ('center_staff', 'quotes.read', 'managed'),
    ('center_staff', 'quotes.write', 'managed'),
    ('center_staff', 'leads.convert_org', 'brand'),
    ('center_social', 'leads.read', 'all'),
    ('center_social', 'leads.write', 'all'),
    ('center_social', 'quotes.read', 'managed'),
    ('center_social', 'quotes.write', 'managed'),
    ('center_social', 'leads.convert_org', 'brand'),
    ('distributor_owner', 'leads.read', 'managed'),
    ('distributor_owner', 'leads.write', 'managed'),
    ('distributor_owner', 'quotes.read', 'managed'),
    ('distributor_owner', 'quotes.write', 'managed'),
    ('distributor_owner', 'leads.convert_org', 'subtree'),
    ('distributor_staff', 'leads.read', 'managed'),
    ('distributor_staff', 'leads.write', 'managed'),
    ('distributor_staff', 'quotes.read', 'managed'),
    ('distributor_staff', 'quotes.write', 'managed'),
    ('dealer_owner', 'leads.read', 'managed'),
    ('dealer_owner', 'leads.write', 'managed'),
    ('dealer_owner', 'quotes.read', 'managed'),
    ('dealer_owner', 'quotes.write', 'managed'),
    ('dealer_staff', 'leads.read', 'managed'),
    ('dealer_staff', 'leads.write', 'managed'),
    ('dealer_staff', 'quotes.read', 'managed'),
    ('dealer_staff', 'quotes.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
