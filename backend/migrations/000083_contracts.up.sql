-- TEC-285 (F3-01a): vehicle intake / service sale contracts. Schema,
-- permissions and the services.contract_id foreign key.
--
--   * contract_templates: definitions made by the center. kind is
--     vehicle_intake or service_sale; one default template per brand and
--     kind (uq_contract_templates_default; the center is the single owner of
--     a brand's templates, so the brand is the uniqueness key). A default
--     template is always active.
--   * contract_template_locales: Lexical JSON + rendered HTML per locale
--     (13 locales, K10) with a version number that the use case bumps on
--     every content change; (template_id, locale) is unique.
--   * contract_instances: one contract of a subject (service or service
--     subscription). The template, locale, version and the OTP / signature
--     requirements are frozen at creation; the rendered HTML and its sha256
--     are the evidence of the executed text. contract_no is a sequential
--     number per organization (contract_counters, same pattern as
--     barcode_counters). Status: draft -> pending -> executed, voided from
--     any non-final state or from executed; voided is final and the frozen
--     content cannot change after execution.
--   * contract_signers: two fixed slots, customer and staff (CHECK, not
--     configurable). The OTP proof is the existing otp_codes row of type
--     contract_sign (F0-11); no separate signer OTP table.
--   * contract_signatures: canvas PNG evidence (storage key, sha256, IP,
--     UA); append-only.
--   * contract_media: images attached to the contract (<= 12 MB each).
--   Signers, signatures and media are locked once the instance is executed
--   or voided.
--   * services.contract_id: placeholder since 000050; now an organization
--     consistent FK (a service links only a contract of its own
--     organization).

-- 1. Templates.
CREATE TABLE contract_templates (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name                VARCHAR(150)  NOT NULL,
    kind                VARCHAR(32)   NOT NULL,
    is_default          BOOLEAN       NOT NULL DEFAULT false,
    otp_required        BOOLEAN       NOT NULL DEFAULT true,
    signature_required  BOOLEAN       NOT NULL DEFAULT true,
    is_active           BOOLEAN       NOT NULL DEFAULT true,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    updated_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_templates_uuid UNIQUE (uuid),
    -- Target of the brand-consistent instance FK.
    CONSTRAINT uq_contract_templates_id_brand UNIQUE (id, brand_id),
    -- Target of the organization/brand-consistent locale FK.
    CONSTRAINT uq_contract_templates_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_contract_templates_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_contract_templates_kind CHECK (kind IN ('vehicle_intake', 'service_sale')),
    CONSTRAINT chk_contract_templates_default_active CHECK (NOT is_default OR is_active)
);

-- One default template per brand and kind.
CREATE UNIQUE INDEX uq_contract_templates_default
    ON contract_templates (brand_id, kind) WHERE is_default;
CREATE INDEX idx_contract_templates_org ON contract_templates (organization_id, kind, name);

CREATE TRIGGER trg_contract_templates_set_updated_at
    BEFORE UPDATE ON contract_templates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- brand_id must be the brand of the organization (000048 helper).
CREATE TRIGGER trg_contract_templates_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON contract_templates
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 2. Template content per locale.
CREATE TABLE contract_template_locales (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    template_id         BIGINT        NOT NULL,
    organization_id     BIGINT        NOT NULL,
    brand_id            BIGINT        NOT NULL,
    locale              VARCHAR(8)    NOT NULL,
    lexical_json        JSONB         NULL,
    html                TEXT          NOT NULL,
    version             INT           NOT NULL DEFAULT 1,
    updated_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_template_locales_uuid UNIQUE (uuid),
    CONSTRAINT uq_contract_template_locales_template_locale UNIQUE (template_id, locale),
    CONSTRAINT fk_contract_template_locales_template FOREIGN KEY (template_id, organization_id, brand_id)
        REFERENCES contract_templates (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_contract_template_locales_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
    ),
    CONSTRAINT chk_contract_template_locales_html CHECK (btrim(html) <> ''),
    CONSTRAINT chk_contract_template_locales_lexical CHECK (lexical_json IS NULL OR jsonb_typeof(lexical_json) = 'object'),
    CONSTRAINT chk_contract_template_locales_version CHECK (version > 0)
);

CREATE TRIGGER trg_contract_template_locales_set_updated_at
    BEFORE UPDATE ON contract_template_locales
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 3. Contract number counter per organization. The use case takes the next
-- number with NextContractNo (one atomic upsert), inside the transaction
-- that inserts the instance.
CREATE TABLE contract_counters (
    organization_id  BIGINT  PRIMARY KEY REFERENCES organizations (id) ON DELETE RESTRICT,
    next_seq         BIGINT  NOT NULL DEFAULT 1,
    CONSTRAINT chk_contract_counters_next CHECK (next_seq >= 1)
);

-- 4. Instances.
CREATE TABLE contract_instances (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    contract_no         BIGINT        NOT NULL,
    -- Subject. service_subscription has no table yet (F3 service catalog);
    -- a service subject is a real organization-consistent FK through the
    -- generated column below.
    subject_type        VARCHAR(32)   NOT NULL,
    subject_id          BIGINT        NOT NULL,
    subject_service_id  BIGINT        GENERATED ALWAYS AS (
        CASE WHEN subject_type = 'service' THEN subject_id END
    ) STORED,
    -- Template snapshot.
    template_id         BIGINT        NOT NULL,
    kind                VARCHAR(32)   NOT NULL,
    locale              VARCHAR(8)    NOT NULL,
    template_version    INT           NOT NULL,
    otp_required        BOOLEAN       NOT NULL,
    signature_required  BOOLEAN       NOT NULL,
    -- Status and evidence.
    status              VARCHAR(16)   NOT NULL DEFAULT 'draft',
    rendered_html       TEXT          NULL,
    content_sha256      CHAR(64)      NULL,
    pdf_key             TEXT          NULL,
    executed_at         TIMESTAMPTZ   NULL,
    voided_at           TIMESTAMPTZ   NULL,
    void_reason         TEXT          NULL,
    voided_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_instances_uuid UNIQUE (uuid),
    CONSTRAINT uq_contract_instances_org_no UNIQUE (organization_id, contract_no),
    -- Target of services.contract_id (organization-consistent).
    CONSTRAINT uq_contract_instances_id_org UNIQUE (id, organization_id),
    -- Target of the organization/brand-consistent FKs of the child tables.
    CONSTRAINT uq_contract_instances_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_contract_instances_template FOREIGN KEY (template_id, brand_id)
        REFERENCES contract_templates (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_contract_instances_subject_service FOREIGN KEY (subject_service_id, organization_id)
        REFERENCES services (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_contract_instances_no CHECK (contract_no >= 1),
    CONSTRAINT chk_contract_instances_subject_type CHECK (subject_type IN ('service', 'service_subscription')),
    CONSTRAINT chk_contract_instances_kind CHECK (kind IN ('vehicle_intake', 'service_sale')),
    CONSTRAINT chk_contract_instances_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
    ),
    CONSTRAINT chk_contract_instances_version CHECK (template_version > 0),
    CONSTRAINT chk_contract_instances_status CHECK (status IN ('draft', 'pending', 'executed', 'voided')),
    CONSTRAINT chk_contract_instances_sha256 CHECK (content_sha256 IS NULL OR content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_contract_instances_pdf_key CHECK (pdf_key IS NULL OR btrim(pdf_key) <> ''),
    -- Executed: the rendered text and its hash are the evidence.
    CONSTRAINT chk_contract_instances_executed CHECK (
        status NOT IN ('draft', 'pending') OR executed_at IS NULL
    ),
    CONSTRAINT chk_contract_instances_evidence CHECK (
        executed_at IS NULL OR (rendered_html IS NOT NULL AND content_sha256 IS NOT NULL)
    ),
    CONSTRAINT chk_contract_instances_status_executed CHECK (status <> 'executed' OR executed_at IS NOT NULL),
    CONSTRAINT chk_contract_instances_voided CHECK (
        (status = 'voided') = (voided_at IS NOT NULL)
        AND (voided_at IS NULL OR (void_reason IS NOT NULL AND btrim(void_reason) <> ''))
    )
);

CREATE INDEX idx_contract_instances_org_created ON contract_instances (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_contract_instances_org_status ON contract_instances (organization_id, status, created_at DESC);
CREATE INDEX idx_contract_instances_subject ON contract_instances (subject_type, subject_id);
CREATE INDEX idx_contract_instances_template ON contract_instances (template_id);

CREATE TRIGGER trg_contract_instances_set_updated_at
    BEFORE UPDATE ON contract_instances
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_contract_instances_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON contract_instances
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- The kind snapshot equals the template kind; status transitions follow
-- draft -> pending -> executed -> voided (voided also from draft/pending,
-- pending back to draft); voided is final; after execution the frozen
-- content (subject, template snapshot, rendered HTML, hash, number) stays.
-- pdf_key may be set once after execution (the PDF renders later).
CREATE FUNCTION contract_instances_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    t_kind VARCHAR(32);
BEGIN
    IF TG_OP = 'INSERT' OR NEW.template_id IS DISTINCT FROM OLD.template_id
       OR NEW.kind IS DISTINCT FROM OLD.kind THEN
        SELECT kind INTO t_kind FROM contract_templates WHERE id = NEW.template_id;
        IF FOUND AND t_kind IS DISTINCT FROM NEW.kind THEN
            RAISE EXCEPTION 'contract_instances: kind % does not match template % (kind %)',
                NEW.kind, NEW.template_id, t_kind
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    IF TG_OP = 'INSERT' THEN
        RETURN NEW;
    END IF;

    IF OLD.status = 'voided' THEN
        RAISE EXCEPTION 'contract_instances: contract % is voided (final)', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND NOT (
        (OLD.status = 'draft' AND NEW.status IN ('pending', 'executed', 'voided'))
        OR (OLD.status = 'pending' AND NEW.status IN ('draft', 'executed', 'voided'))
        OR (OLD.status = 'executed' AND NEW.status = 'voided')
    ) THEN
        RAISE EXCEPTION 'contract_instances: transition % -> % is not allowed (contract %)',
            OLD.status, NEW.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF OLD.status = 'executed' AND (
        NEW.organization_id IS DISTINCT FROM OLD.organization_id
        OR NEW.brand_id IS DISTINCT FROM OLD.brand_id
        OR NEW.contract_no IS DISTINCT FROM OLD.contract_no
        OR NEW.subject_type IS DISTINCT FROM OLD.subject_type
        OR NEW.subject_id IS DISTINCT FROM OLD.subject_id
        OR NEW.template_id IS DISTINCT FROM OLD.template_id
        OR NEW.kind IS DISTINCT FROM OLD.kind
        OR NEW.locale IS DISTINCT FROM OLD.locale
        OR NEW.template_version IS DISTINCT FROM OLD.template_version
        OR NEW.otp_required IS DISTINCT FROM OLD.otp_required
        OR NEW.signature_required IS DISTINCT FROM OLD.signature_required
        OR NEW.rendered_html IS DISTINCT FROM OLD.rendered_html
        OR NEW.content_sha256 IS DISTINCT FROM OLD.content_sha256
        OR NEW.executed_at IS DISTINCT FROM OLD.executed_at
        OR (OLD.pdf_key IS NOT NULL AND NEW.pdf_key IS DISTINCT FROM OLD.pdf_key)
    ) THEN
        RAISE EXCEPTION 'contract_instances: contract % is executed; its content is frozen', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_contract_instances_check_row
    BEFORE INSERT OR UPDATE ON contract_instances
    FOR EACH ROW
    EXECUTE FUNCTION contract_instances_check_row();

-- Executed and voided contracts are records; they are never deleted.
CREATE FUNCTION contract_instances_no_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status IN ('executed', 'voided') THEN
        RAISE EXCEPTION 'contract_instances: contract % is %; it cannot be deleted', OLD.id, OLD.status
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN OLD;
END;
$$;

CREATE TRIGGER trg_contract_instances_no_delete
    BEFORE DELETE ON contract_instances
    FOR EACH ROW
    EXECUTE FUNCTION contract_instances_no_delete();

-- Signers, signatures and media change only while the contract is open
-- (draft or pending).
CREATE FUNCTION contract_children_check_open() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    c_status VARCHAR(16);
    iid      BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        iid := OLD.instance_id;
    ELSE
        iid := NEW.instance_id;
    END IF;
    SELECT status INTO c_status FROM contract_instances WHERE id = iid;
    IF c_status IN ('executed', 'voided') THEN
        RAISE EXCEPTION '%: contract % is %; it is locked', TG_TABLE_NAME, iid, c_status
            USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.instance_id IS DISTINCT FROM NEW.instance_id THEN
        SELECT status INTO c_status FROM contract_instances WHERE id = OLD.instance_id;
        IF c_status IN ('executed', 'voided') THEN
            RAISE EXCEPTION '%: contract % is %; it is locked', TG_TABLE_NAME, OLD.instance_id, c_status
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

-- 5. Signers: two fixed slots per contract.
CREATE TABLE contract_signers (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    instance_id         BIGINT        NOT NULL,
    organization_id     BIGINT        NOT NULL,
    brand_id            BIGINT        NOT NULL,
    role                VARCHAR(16)   NOT NULL,
    user_id             BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    name                VARCHAR(255)  NOT NULL,
    phone_e164          VARCHAR(20)   NULL,
    -- OTP proof: the consumed otp_codes row of type contract_sign.
    otp_code_id         BIGINT        NULL REFERENCES otp_codes (id) ON DELETE RESTRICT,
    otp_verified_at     TIMESTAMPTZ   NULL,
    signed_at           TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_signers_uuid UNIQUE (uuid),
    CONSTRAINT uq_contract_signers_instance_role UNIQUE (instance_id, role),
    -- Target of the signature FK (the signer belongs to the contract).
    CONSTRAINT uq_contract_signers_id_instance UNIQUE (id, instance_id),
    CONSTRAINT fk_contract_signers_instance FOREIGN KEY (instance_id, organization_id, brand_id)
        REFERENCES contract_instances (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_contract_signers_role CHECK (role IN ('customer', 'staff')),
    CONSTRAINT chk_contract_signers_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_contract_signers_phone CHECK (phone_e164 IS NULL OR phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT chk_contract_signers_otp CHECK ((otp_code_id IS NULL) = (otp_verified_at IS NULL))
);

CREATE INDEX idx_contract_signers_user ON contract_signers (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_contract_signers_otp ON contract_signers (otp_code_id) WHERE otp_code_id IS NOT NULL;

CREATE TRIGGER trg_contract_signers_set_updated_at
    BEFORE UPDATE ON contract_signers
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_contract_signers_check_open
    BEFORE INSERT OR UPDATE OR DELETE ON contract_signers
    FOR EACH ROW
    EXECUTE FUNCTION contract_children_check_open();

-- The OTP proof must be a consumed contract_sign code.
CREATE FUNCTION contract_signers_check_otp() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type     VARCHAR(32);
    o_consumed TIMESTAMPTZ;
BEGIN
    IF NEW.otp_code_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.otp_code_id IS NOT DISTINCT FROM OLD.otp_code_id THEN
        RETURN NEW;
    END IF;
    SELECT type, consumed_at INTO o_type, o_consumed FROM otp_codes WHERE id = NEW.otp_code_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF o_type <> 'contract_sign' OR o_consumed IS NULL THEN
        RAISE EXCEPTION 'contract_signers: otp code % is not a consumed contract_sign code', NEW.otp_code_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_contract_signers_check_otp
    BEFORE INSERT OR UPDATE OF otp_code_id ON contract_signers
    FOR EACH ROW
    EXECUTE FUNCTION contract_signers_check_otp();

-- 6. Signatures (append-only evidence).
CREATE TABLE contract_signatures (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    signer_id        BIGINT        NOT NULL,
    instance_id      BIGINT        NOT NULL,
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    storage_key      TEXT          NOT NULL,
    sha256           CHAR(64)      NOT NULL,
    ip_address       INET          NULL,
    user_agent       VARCHAR(512)  NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_signatures_uuid UNIQUE (uuid),
    CONSTRAINT fk_contract_signatures_signer FOREIGN KEY (signer_id, instance_id)
        REFERENCES contract_signers (id, instance_id) ON DELETE RESTRICT,
    CONSTRAINT fk_contract_signatures_instance FOREIGN KEY (instance_id, organization_id, brand_id)
        REFERENCES contract_instances (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_contract_signatures_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_contract_signatures_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_contract_signatures_signer ON contract_signatures (signer_id, created_at, id);
CREATE INDEX idx_contract_signatures_instance ON contract_signatures (instance_id, created_at, id);

CREATE TRIGGER trg_contract_signatures_check_open
    BEFORE INSERT ON contract_signatures
    FOR EACH ROW
    EXECUTE FUNCTION contract_children_check_open();

CREATE FUNCTION contract_signatures_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'contract_signatures is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_contract_signatures_append_only
    BEFORE UPDATE OR DELETE ON contract_signatures
    FOR EACH ROW
    EXECUTE FUNCTION contract_signatures_append_only();

CREATE TRIGGER trg_contract_signatures_no_truncate
    BEFORE TRUNCATE ON contract_signatures
    FOR EACH STATEMENT
    EXECUTE FUNCTION contract_signatures_append_only();

-- 7. Media (images attached to the contract, at most 12 MB each).
CREATE TABLE contract_media (
    id                   BIGSERIAL     PRIMARY KEY,
    uuid                 UUID          NOT NULL DEFAULT gen_random_uuid(),
    instance_id          BIGINT        NOT NULL,
    organization_id      BIGINT        NOT NULL,
    brand_id             BIGINT        NOT NULL,
    storage_key          TEXT          NOT NULL,
    mime_type            VARCHAR(100)  NOT NULL,
    size_bytes           BIGINT        NOT NULL,
    sha256               CHAR(64)      NOT NULL,
    title                VARCHAR(255)  NULL,
    sort_order           INT           NOT NULL DEFAULT 0,
    uploaded_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_media_uuid UNIQUE (uuid),
    CONSTRAINT fk_contract_media_instance FOREIGN KEY (instance_id, organization_id, brand_id)
        REFERENCES contract_instances (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_contract_media_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_contract_media_mime CHECK (btrim(mime_type) <> ''),
    CONSTRAINT chk_contract_media_size CHECK (size_bytes > 0 AND size_bytes <= 12582912),
    CONSTRAINT chk_contract_media_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_contract_media_instance ON contract_media (instance_id, sort_order, id);

CREATE TRIGGER trg_contract_media_check_open
    BEFORE INSERT OR UPDATE OR DELETE ON contract_media
    FOR EACH ROW
    EXECUTE FUNCTION contract_children_check_open();

-- 8. services.contract_id (placeholder since 000050) becomes an
-- organization-consistent FK. No contract existed before this migration,
-- so any value already there is dangling and is cleared.
UPDATE services SET contract_id = NULL WHERE contract_id IS NOT NULL;

ALTER TABLE services
    ADD CONSTRAINT fk_services_contract FOREIGN KEY (contract_id, organization_id)
        REFERENCES contract_instances (id, organization_id) ON DELETE RESTRICT;

CREATE INDEX idx_services_contract ON services (contract_id) WHERE contract_id IS NOT NULL;

-- 9. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (TestMigrationMatchesCatalog compares the two); appended last, sort order
-- MAX + 10. templates.manage and void are center-only (scopes brand/all);
-- read and write follow services.read / services.write of the roles that
-- open services.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage contract templates', 'contracts.templates.manage', 'contracts',
       ARRAY['brand', 'all']::text[], false, false,
       'Create and edit contract templates, their locale texts and the default template per kind (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read contracts', 'contracts.read', 'contracts',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all', 'customer']::text[], false, false,
       'Vehicle intake and service sale contracts with their signers, signatures and media.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write contracts', 'contracts.write', 'contracts',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Create contracts for services, attach media, collect OTP and signatures and execute them.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Void contracts', 'contracts.void', 'contracts',
       ARRAY['brand', 'all']::text[], false, false,
       'Void a contract, also an executed one, with a reason (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'contracts.templates.manage', 'all'),
    ('super_admin', 'contracts.read', 'all'),
    ('super_admin', 'contracts.write', 'all'),
    ('super_admin', 'contracts.void', 'all'),
    ('center_staff', 'contracts.templates.manage', 'brand'),
    ('center_staff', 'contracts.read', 'brand'),
    ('center_staff', 'contracts.write', 'brand'),
    ('center_staff', 'contracts.void', 'brand'),
    ('distributor_owner', 'contracts.read', 'subtree'),
    ('distributor_owner', 'contracts.write', 'subtree'),
    ('distributor_staff', 'contracts.read', 'subtree'),
    ('distributor_staff', 'contracts.write', 'subtree'),
    ('dealer_owner', 'contracts.read', 'managed'),
    ('dealer_owner', 'contracts.write', 'managed'),
    ('dealer_staff', 'contracts.read', 'managed'),
    ('dealer_staff', 'contracts.write', 'own')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
