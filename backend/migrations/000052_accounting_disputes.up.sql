-- TEC-174 (F1-07d): cari disputes (K24). A lower organization never writes
-- or deletes what its parent posted to its ledger; it opens a dispute on the
-- row with a reason. The parent resolves it in one of three ways:
--
--   resolved_reversal  every open row of the source is reversed
--                      (posting.VoidBySourceTx, reversal_of_id rows);
--   resolved_revision  the source is reversed and reposted with a corrected
--                      amount as revision + 1, in the same transaction;
--   rejected           nothing is posted; resolution_note says why.
--
-- organization_id is the disputing (child) organization that owns the
-- disputed ledger row; counterparty_org_id is its parent, the organization
-- the row came from and the one that resolves the dispute. Final statuses
-- never change; one open dispute per ledger row.

CREATE TABLE accounting_disputes (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id             BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    counterparty_org_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    entry_id             BIGINT         NOT NULL REFERENCES finance_entries (id) ON DELETE RESTRICT,
    source_type          VARCHAR(64)    NOT NULL,
    source_uuid          UUID           NOT NULL,
    status               VARCHAR(24)    NOT NULL DEFAULT 'open',
    reason               TEXT           NOT NULL,
    -- Corrected amount of a revision, in the disputed row's orig_currency.
    corrected_amount     NUMERIC(18,2)  NULL,
    resolution_note      TEXT           NULL,
    reversal_entry_id    BIGINT         NULL REFERENCES finance_entries (id) ON DELETE RESTRICT,
    revision_entry_id    BIGINT         NULL REFERENCES finance_entries (id) ON DELETE RESTRICT,
    opened_by_user_id    BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    resolved_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    resolved_at          TIMESTAMPTZ    NULL,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_accounting_disputes_uuid UNIQUE (uuid),
    CONSTRAINT chk_accounting_disputes_status CHECK (status IN (
        'open', 'resolved_reversal', 'resolved_revision', 'rejected'
    )),
    CONSTRAINT chk_accounting_disputes_not_self CHECK (counterparty_org_id <> organization_id),
    CONSTRAINT chk_accounting_disputes_reason CHECK (btrim(reason) <> '' AND char_length(reason) <= 2000),
    CONSTRAINT chk_accounting_disputes_note CHECK (
        resolution_note IS NULL OR (btrim(resolution_note) <> '' AND char_length(resolution_note) <= 2000)
    ),
    CONSTRAINT chk_accounting_disputes_amount CHECK (corrected_amount IS NULL OR corrected_amount > 0),
    CONSTRAINT chk_accounting_disputes_resolved CHECK (
        (status = 'open') = (resolved_at IS NULL)
    ),
    CONSTRAINT chk_accounting_disputes_resolution CHECK (
        CASE status
            WHEN 'open' THEN reversal_entry_id IS NULL AND revision_entry_id IS NULL
                AND corrected_amount IS NULL AND resolution_note IS NULL
            WHEN 'resolved_reversal' THEN reversal_entry_id IS NOT NULL AND revision_entry_id IS NULL
                AND corrected_amount IS NULL
            WHEN 'resolved_revision' THEN reversal_entry_id IS NOT NULL AND revision_entry_id IS NOT NULL
                AND corrected_amount IS NOT NULL
            ELSE reversal_entry_id IS NULL AND revision_entry_id IS NULL
                AND corrected_amount IS NULL AND resolution_note IS NOT NULL
        END
    )
);

-- One open dispute per ledger row.
CREATE UNIQUE INDEX uq_accounting_disputes_open_entry
    ON accounting_disputes (entry_id) WHERE status = 'open';
CREATE INDEX idx_accounting_disputes_org_created ON accounting_disputes (organization_id, created_at);
CREATE INDEX idx_accounting_disputes_counterparty ON accounting_disputes (counterparty_org_id, status, created_at);
CREATE INDEX idx_accounting_disputes_brand_created ON accounting_disputes (brand_id, created_at);

CREATE TRIGGER trg_accounting_disputes_set_updated_at
    BEFORE UPDATE ON accounting_disputes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Insert guard: brand follows the organization; the counterparty is the
-- organization's parent in the same brand (K9: the supplier is one level
-- up); the row belongs to the disputer's cari with that parent, is an
-- original (not a reversal) sourced row written by another module (not a
-- manual entry of the disputer's own) and the source is copied from it.
--
-- Update guard: final statuses never change; identity columns never change;
-- the reversal row reverses the disputed row and the revision row is a
-- later revision of the same source in the disputer's ledger.
CREATE FUNCTION accounting_disputes_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand    BIGINT;
    org_parent   BIGINT;
    cp_brand     BIGINT;
    e            finance_entries%ROWTYPE;
    cari_cp      BIGINT;
    r            finance_entries%ROWTYPE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'open' THEN
            RAISE EXCEPTION 'accounting_disputes: dispute % is final (%) and cannot be deleted', OLD.id, OLD.status
                USING ERRCODE = 'restrict_violation';
        END IF;
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF OLD.status <> 'open' THEN
            RAISE EXCEPTION 'accounting_disputes: dispute % is final (%)', OLD.id, OLD.status
                USING ERRCODE = 'restrict_violation';
        END IF;
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id
           OR NEW.counterparty_org_id <> OLD.counterparty_org_id
           OR NEW.entry_id <> OLD.entry_id
           OR NEW.source_type <> OLD.source_type
           OR NEW.source_uuid <> OLD.source_uuid
           OR NEW.reason <> OLD.reason
           OR NEW.opened_by_user_id IS DISTINCT FROM OLD.opened_by_user_id
           OR NEW.created_at <> OLD.created_at THEN
            RAISE EXCEPTION 'accounting_disputes: identity of dispute % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        IF NEW.reversal_entry_id IS NOT NULL THEN
            SELECT * INTO r FROM finance_entries WHERE id = NEW.reversal_entry_id;
            IF FOUND AND r.reversal_of_id IS DISTINCT FROM NEW.entry_id THEN
                RAISE EXCEPTION 'accounting_disputes: entry % does not reverse disputed entry %',
                    NEW.reversal_entry_id, NEW.entry_id
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
        IF NEW.revision_entry_id IS NOT NULL THEN
            SELECT * INTO e FROM finance_entries WHERE id = NEW.entry_id;
            SELECT * INTO r FROM finance_entries WHERE id = NEW.revision_entry_id;
            IF FOUND AND (r.reversal_of_id IS NOT NULL
                          OR r.organization_id <> NEW.organization_id
                          OR r.source_type IS DISTINCT FROM NEW.source_type
                          OR r.source_uuid IS DISTINCT FROM NEW.source_uuid
                          OR r.role <> e.role
                          OR r.revision <= e.revision) THEN
                RAISE EXCEPTION 'accounting_disputes: entry % is not a revision of disputed entry %',
                    NEW.revision_entry_id, NEW.entry_id
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
        RETURN NEW;
    END IF;

    -- INSERT
    IF NEW.status <> 'open' THEN
        RAISE EXCEPTION 'accounting_disputes: a dispute is opened with status open'
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT brand_id, parent_id INTO org_brand, org_parent
    FROM organizations WHERE id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'accounting_disputes: brand % does not match organization % (brand %)',
            NEW.brand_id, NEW.organization_id, org_brand
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT brand_id INTO cp_brand FROM organizations WHERE id = NEW.counterparty_org_id;
    IF FOUND AND cp_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'accounting_disputes: counterparty % belongs to brand %, dispute to brand %',
            NEW.counterparty_org_id, cp_brand, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF org_parent IS DISTINCT FROM NEW.counterparty_org_id THEN
        RAISE EXCEPTION 'accounting_disputes: counterparty % is not the parent of organization %',
            NEW.counterparty_org_id, NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;

    SELECT * INTO e FROM finance_entries WHERE id = NEW.entry_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    SELECT counterparty_org_id INTO cari_cp FROM cari_accounts WHERE id = e.cari_id;
    IF e.organization_id <> NEW.organization_id
       OR e.cari_id IS NULL
       OR cari_cp IS DISTINCT FROM NEW.counterparty_org_id THEN
        RAISE EXCEPTION 'accounting_disputes: entry % is not on the cari of organization % with %',
            e.id, NEW.organization_id, NEW.counterparty_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF e.reversal_of_id IS NOT NULL
       OR e.source_type IS NULL
       OR e.source_type = 'manual'
       OR e.source_type <> NEW.source_type
       OR e.source_uuid <> NEW.source_uuid THEN
        RAISE EXCEPTION 'accounting_disputes: entry % is not a sourced original row posted by the parent', e.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_accounting_disputes_check_row
    BEFORE INSERT OR UPDATE OR DELETE ON accounting_disputes
    FOR EACH ROW
    EXECUTE FUNCTION accounting_disputes_check_row();

-- Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last). accounting.dispute (000047) opens a dispute; the parent
-- resolves it with accounting.resolve.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Resolve accounting disputes', 'accounting.resolve', 'accounting',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Resolve a dispute on an entry posted to a child: reversal, revision or rejection (K24).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'accounting.resolve', 'all'),
    ('center_accounting', 'accounting.resolve', 'brand'),
    ('distributor_owner', 'accounting.resolve', 'managed'),
    ('distributor_accounting', 'accounting.resolve', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
