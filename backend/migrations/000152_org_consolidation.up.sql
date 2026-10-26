-- TEC-1003: organization consolidation. Türkiye has no distributor of its
-- own any more: the TR dealers hang directly under the brand center, and the
-- center's own service point (AFM Florya, same legal entity) is merged into
-- the center instead of being a dealer organization of its own.
--
-- 1. Service point: the name and logo the center shows on customer surfaces
--    (warranty page and PDF, portal, WhatsApp) while the panel keeps the
--    organization name. service_point_enabled turns the center's own service
--    work on (own-services vs. network views, on-behalf service creation).
ALTER TABLE organizations
    ADD COLUMN service_point_enabled         BOOLEAN      NOT NULL DEFAULT FALSE,
    ADD COLUMN service_point_name            VARCHAR(200) NULL,
    ADD COLUMN service_point_logo_object_key TEXT         NULL,
    -- A merged organization is soft deleted (deleted_at) and points at the
    -- organization that took over its records; history rows that stay with
    -- it (e.g. legacy orders it bought) still resolve its name.
    ADD COLUMN merged_into_id                BIGINT       NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_organizations_service_point_name CHECK (
        service_point_name IS NULL OR btrim(service_point_name) <> ''),
    ADD CONSTRAINT chk_organizations_merged_into CHECK (
        merged_into_id IS NULL OR (merged_into_id <> id AND deleted_at IS NOT NULL));

CREATE INDEX idx_organizations_merged_into ON organizations (merged_into_id) WHERE merged_into_id IS NOT NULL;

-- 2. Merge runs (cmd/org-merge and the migrator organizations step). Every
--    changed row is recorded so a run can be undone: 'update' rows had
--    column moved from source to target, 'insert' rows were added (target
--    memberships), 'delete' rows keep the removed row in row_data.
CREATE TABLE org_merge_runs (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    source_org_id    BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    target_org_id    BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    status           VARCHAR(16)  NOT NULL DEFAULT 'applied',
    -- Who ran it: "cli:<user>", "migrator", ...
    actor            VARCHAR(128) NOT NULL,
    -- Plan / result report (per-table counts, balances before/after) and
    -- the organization fields the run changed (restored by undo).
    report           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    previous         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    undone_at        TIMESTAMPTZ  NULL,
    CONSTRAINT uq_org_merge_runs_uuid UNIQUE (uuid),
    CONSTRAINT chk_org_merge_runs_status CHECK (status IN ('applied', 'undone')),
    CONSTRAINT chk_org_merge_runs_orgs CHECK (source_org_id <> target_org_id)
);

CREATE INDEX idx_org_merge_runs_source ON org_merge_runs (source_org_id, created_at DESC);

CREATE TABLE org_merge_changes (
    id          BIGSERIAL    PRIMARY KEY,
    run_id      BIGINT       NOT NULL REFERENCES org_merge_runs (id) ON DELETE CASCADE,
    kind        VARCHAR(8)   NOT NULL,
    table_name  VARCHAR(63)  NOT NULL,
    column_name VARCHAR(63)  NULL,
    -- Primary key of the row ({"id": 12} or the composite key columns).
    pk          JSONB        NOT NULL,
    row_data    JSONB        NULL,
    CONSTRAINT chk_org_merge_changes_kind CHECK (kind IN ('update', 'insert', 'delete')),
    CONSTRAINT chk_org_merge_changes_column CHECK ((kind = 'update') = (column_name IS NOT NULL)),
    CONSTRAINT chk_org_merge_changes_row CHECK ((kind = 'delete') = (row_data IS NOT NULL))
);

CREATE INDEX idx_org_merge_changes_run ON org_merge_changes (run_id, table_name, kind);

-- 3. A merge moves a parent row and its children in one transaction: both
--    carry the organization column inside a composite foreign key, so the
--    keys are made deferrable (still checked immediately unless a
--    transaction defers them).
DO $$
DECLARE r record;
BEGIN
    FOR r IN
        SELECT c.conname, c.conrelid::regclass AS tbl
        FROM pg_constraint c
        JOIN pg_namespace n ON n.oid = c.connamespace
        WHERE c.contype = 'f' AND n.nspname = 'public' AND NOT c.condeferrable
          AND (c.confrelid = 'organizations'::regclass OR array_length(c.conkey, 1) > 1)
    LOOP
        EXECUTE format('ALTER TABLE %s ALTER CONSTRAINT %I DEFERRABLE INITIALLY IMMEDIATE', r.tbl, r.conname);
    END LOOP;
END $$;

-- 4. A merge re-points the organization column of rows that row triggers
--    guard: append-only ledgers (stock movements, finance entries, status
--    logs), "owner cannot change" and "completed / locked" checks. The same
--    legal entity takes the records over, so those trigger functions let an
--    UPDATE through while the transaction-local setting olex.org_merge is
--    on; only internal/platform/orgmerge sets it, like
--    olex.legacy_messages_anonymize for K19. The order parties check (K6)
--    and functions that derive data keep running.
DO $$
DECLARE r record; def text;
BEGIN
    FOR r IN
        SELECT DISTINCT p.oid
        FROM pg_trigger t
        JOIN pg_proc p ON p.oid = t.tgfoid
        JOIN pg_class c ON c.oid = t.tgrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND NOT t.tgisinternal
          AND (t.tgtype & 16) <> 0          -- UPDATE
          AND (t.tgtype & 1) <> 0           -- FOR EACH ROW
          AND p.prosrc LIKE '%RAISE EXCEPTION%'
          AND p.proname NOT IN ('orders_check_parties', 'warehouse_locations_derive',
                                'check_role_permission_scope', 'organizations_check_fleet')
    LOOP
        def := pg_get_functiondef(r.oid);
        IF position('olex.org_merge' IN def) = 0 THEN
            def := regexp_replace(def, '\mBEGIN\M',
                E'BEGIN\n    IF TG_OP = ''UPDATE'' AND current_setting(''olex.org_merge'', true) = ''on'' THEN RETURN NEW; END IF; -- TEC-1003 org merge');
            EXECUTE def;
        END IF;
    END LOOP;
END $$;

-- 5. K6 (TEC-1003): a dealer directly under the center (Türkiye) buys from
--    the center. Same function as 000049 with the center -> dealer flow.
CREATE OR REPLACE FUNCTION orders_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    s_type   TEXT;
    s_brand  BIGINT;
    b_type   TEXT;
    b_brand  BIGINT;
    b_parent BIGINT;
    b_cur    TEXT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.seller_org_id = OLD.seller_org_id
       AND NEW.buyer_org_id = OLD.buyer_org_id
       AND NEW.brand_id = OLD.brand_id
       AND NEW.currency = OLD.currency THEN
        RETURN NEW;
    END IF;
    -- chk_orders_seller_not_buyer reports this case.
    IF NEW.seller_org_id = NEW.buyer_org_id THEN
        RETURN NEW;
    END IF;

    SELECT type, brand_id INTO s_type, s_brand FROM organizations WHERE id = NEW.seller_org_id;
    SELECT type, brand_id, parent_id INTO b_type, b_brand, b_parent
    FROM organizations WHERE id = NEW.buyer_org_id;
    IF s_brand IS DISTINCT FROM NEW.brand_id OR b_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'order parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF b_parent IS DISTINCT FROM NEW.seller_org_id THEN
        RAISE EXCEPTION 'order buyer % is not a direct child of seller %',
            NEW.buyer_org_id, NEW.seller_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NOT ((s_type = 'center' AND b_type IN ('distributor', 'dealer'))
            OR (s_type = 'distributor' AND b_type = 'dealer')) THEN
        RAISE EXCEPTION 'order flow %->% is not allowed (K6)', s_type, b_type
            USING ERRCODE = 'check_violation';
    END IF;

    IF TG_OP = 'INSERT' OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.brand_id IS DISTINCT FROM OLD.brand_id THEN
        SELECT currency INTO b_cur FROM brands WHERE id = NEW.brand_id;
        IF b_cur IS DISTINCT FROM NEW.currency THEN
            RAISE EXCEPTION 'order currency % must be the brand currency %', NEW.currency, b_cur
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

-- 6. center_service: the center's service point staff (former AFM Florya
--    dealer staff). Same package as dealer_staff, at the center: own
--    services, customers, warranties and appointments; no warehouse,
--    pricing, accounting or network views (the managed scope keeps every
--    grant inside the center's own records).
INSERT INTO roles (name, slug, description, is_system, org_type) VALUES
    ('Center service staff', 'center_service',
     'Center service point staff: the center''s own services, customers, warranties and appointments; no warehouse, prices or accounting',
     TRUE, 'center')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, rp.permission_id, rp.scope
FROM roles r
CROSS JOIN role_permissions rp
JOIN roles src ON src.id = rp.role_id AND src.slug = 'dealer_staff'
JOIN permissions p ON p.id = rp.permission_id
-- The dealer showcase is a dealer page; the center has none.
WHERE r.slug = 'center_service' AND p.slug <> 'showcase.read'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
