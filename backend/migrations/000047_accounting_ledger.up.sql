-- TEC-171 (F1-07a): accounting schema. Decisions: TEC-99 orchestrator
-- comment items 1-8 (one-sided ledger + cari; no double-entry chart).
--
--   * finance_accounts: cash/bank accounts of an organization.
--   * cari_accounts: current account of an organization with a counterparty
--     (another organization; a user/customer later with TEC-118).
--   * finance_entries: the ledger. Append-only (trigger); a correction is a
--     mirror row with reversal_of_id. Every row keeps the original currency
--     and amount, the amount in the organization's currency (K7) and the
--     frozen rate (rate + rate_date, TEC-96 rate_snapshot).
--   * cari_account_balances / finance_account_balances: balance views over
--     the ledger (no stored projection yet; TEC-99b may add one).
--
-- Sign convention. Amounts of an original row are positive; a reversal row
-- carries the negated amounts of the row it reverses, so a plain SUM over
-- the ledger nets it out. Cari balance is "the counterparty owes the
-- organization" (receivable positive):
--   income  (+, a sale on cari)       expense    (-, a purchase on cari)
--   charge  (+, non-P&L debit)        collection (-, counterparty paid us)
--   payment (+, we paid the counterparty)
-- Cash/bank balance: income +, expense -, collection +, payment -; a charge
-- never touches an account.
--
-- Income is written once, at the sale (TEC-99 decision 2); a collection is a
-- cash movement plus cari settlement, never a second income.

-- 1. Cash and bank accounts.
CREATE TABLE finance_accounts (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    type             VARCHAR(8)    NOT NULL,
    name             VARCHAR(200)  NOT NULL,
    currency         CHAR(3)       NOT NULL,
    iban             VARCHAR(34)   NULL,
    active           BOOLEAN       NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_finance_accounts_uuid UNIQUE (uuid),
    CONSTRAINT uq_finance_accounts_id_org UNIQUE (id, organization_id),
    CONSTRAINT chk_finance_accounts_type CHECK (type IN ('cash', 'bank')),
    CONSTRAINT chk_finance_accounts_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_finance_accounts_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_finance_accounts_iban CHECK (
        iban IS NULL OR (type = 'bank' AND iban ~ '^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$')
    )
);

CREATE INDEX idx_finance_accounts_org ON finance_accounts (organization_id, active);

CREATE TRIGGER trg_finance_accounts_set_updated_at
    BEFORE UPDATE ON finance_accounts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 2. Cari (current) accounts. The owner keeps the cari in its own currency
-- (K7). Exactly one counterparty column is set; counterparty_type = 'user'
-- is opened now for TEC-118 (customer cari) without UI.
CREATE TABLE cari_accounts (
    id                    BIGSERIAL     PRIMARY KEY,
    uuid                  UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    counterparty_type     VARCHAR(16)   NOT NULL,
    counterparty_org_id   BIGINT        NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    counterparty_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    currency              CHAR(3)       NOT NULL,
    active                BOOLEAN       NOT NULL DEFAULT true,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cari_accounts_uuid UNIQUE (uuid),
    CONSTRAINT uq_cari_accounts_id_org UNIQUE (id, organization_id),
    CONSTRAINT chk_cari_accounts_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_cari_accounts_counterparty CHECK (
        (counterparty_type = 'organization' AND counterparty_org_id IS NOT NULL AND counterparty_user_id IS NULL)
        OR (counterparty_type = 'user' AND counterparty_user_id IS NOT NULL AND counterparty_org_id IS NULL)
    ),
    CONSTRAINT chk_cari_accounts_not_self CHECK (counterparty_org_id IS DISTINCT FROM organization_id)
);

CREATE UNIQUE INDEX uq_cari_accounts_org_counterparty_org
    ON cari_accounts (organization_id, counterparty_org_id) WHERE counterparty_org_id IS NOT NULL;
CREATE UNIQUE INDEX uq_cari_accounts_org_counterparty_user
    ON cari_accounts (organization_id, counterparty_user_id) WHERE counterparty_user_id IS NOT NULL;
CREATE INDEX idx_cari_accounts_counterparty_org ON cari_accounts (counterparty_org_id)
    WHERE counterparty_org_id IS NOT NULL;

CREATE TRIGGER trg_cari_accounts_set_updated_at
    BEFORE UPDATE ON cari_accounts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Accounts belong to the organization's brand and are kept in its currency.
CREATE FUNCTION accounting_accounts_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand    BIGINT;
    org_currency CHAR(3);
BEGIN
    SELECT brand_id, currency INTO org_brand, org_currency
    FROM organizations WHERE id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION '%: brand % does not match organization % (brand %)',
            TG_TABLE_NAME, NEW.brand_id, NEW.organization_id, org_brand
            USING ERRCODE = 'check_violation';
    END IF;
    IF org_currency IS DISTINCT FROM NEW.currency THEN
        RAISE EXCEPTION '%: currency % differs from organization currency %',
            TG_TABLE_NAME, NEW.currency, org_currency
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_finance_accounts_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id, currency ON finance_accounts
    FOR EACH ROW
    EXECUTE FUNCTION accounting_accounts_check_org();

CREATE TRIGGER trg_cari_accounts_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id, currency ON cari_accounts
    FOR EACH ROW
    EXECUTE FUNCTION accounting_accounts_check_org();

-- 3. Ledger entries (append-only).
--
-- Idempotency: a sourced write (order, service, ...) is keyed by
-- (organization_id, source_type, source_uuid, role, revision); writers
-- INSERT ... ON CONFLICT DO NOTHING and read the existing row back. role
-- separates the rows one source writes in one organization (e.g. 'income'
-- and 'cari'); revision is bumped when a dispute reposts a corrected amount
-- (TEC-99d). Reversals are outside that key and are unique per reversed row.
CREATE TABLE finance_entries (
    id               BIGSERIAL      PRIMARY KEY,
    uuid             UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    account_id       BIGINT         NULL,
    cari_id          BIGINT         NULL,
    direction        VARCHAR(16)    NOT NULL,
    category         VARCHAR(64)    NOT NULL,
    orig_currency    CHAR(3)        NOT NULL,
    orig_amount      NUMERIC(18,2)  NOT NULL,
    currency         CHAR(3)        NOT NULL,
    amount           NUMERIC(18,2)  NOT NULL,
    rate             NUMERIC(18,8)  NOT NULL,
    rate_date        DATE           NOT NULL,
    source_type      VARCHAR(64)    NULL,
    source_uuid      UUID           NULL,
    role             VARCHAR(32)    NOT NULL DEFAULT 'main',
    revision         INT            NOT NULL DEFAULT 1,
    reversal_of_id   BIGINT         NULL REFERENCES finance_entries (id) ON DELETE RESTRICT,
    description      TEXT           NULL,
    actor_user_id    BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_finance_entries_uuid UNIQUE (uuid),
    CONSTRAINT uq_finance_entries_reversal_of UNIQUE (reversal_of_id),
    CONSTRAINT fk_finance_entries_account FOREIGN KEY (account_id, organization_id)
        REFERENCES finance_accounts (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_finance_entries_cari FOREIGN KEY (cari_id, organization_id)
        REFERENCES cari_accounts (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_finance_entries_direction CHECK (direction IN (
        'income', 'expense', 'charge', 'payment', 'collection'
    )),
    -- income/expense hit an account, a cari (credit sale/purchase) or both;
    -- charge is cari only; payment/collection always settle a cari.
    CONSTRAINT chk_finance_entries_targets CHECK (
        CASE direction
            WHEN 'charge' THEN cari_id IS NOT NULL AND account_id IS NULL
            WHEN 'payment' THEN cari_id IS NOT NULL
            WHEN 'collection' THEN cari_id IS NOT NULL
            ELSE account_id IS NOT NULL OR cari_id IS NOT NULL
        END
    ),
    CONSTRAINT chk_finance_entries_category CHECK (category ~ '^[a-z][a-z0-9_.]{0,63}$'),
    CONSTRAINT chk_finance_entries_orig_currency CHECK (orig_currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_finance_entries_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_finance_entries_rate CHECK (rate > 0),
    CONSTRAINT chk_finance_entries_same_currency CHECK (
        orig_currency <> currency OR (rate = 1 AND amount = orig_amount)
    ),
    CONSTRAINT chk_finance_entries_sign CHECK (
        (reversal_of_id IS NULL AND amount > 0 AND orig_amount > 0)
        OR (reversal_of_id IS NOT NULL AND amount < 0 AND orig_amount < 0)
    ),
    CONSTRAINT chk_finance_entries_source CHECK ((source_type IS NULL) = (source_uuid IS NULL)),
    CONSTRAINT chk_finance_entries_source_type CHECK (source_type IS NULL OR source_type ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT chk_finance_entries_role CHECK (role ~ '^[a-z][a-z0-9_]{0,31}$'),
    CONSTRAINT chk_finance_entries_revision CHECK (revision >= 1),
    CONSTRAINT chk_finance_entries_not_self_reversal CHECK (reversal_of_id IS DISTINCT FROM id)
);

CREATE UNIQUE INDEX uq_finance_entries_source
    ON finance_entries (organization_id, source_type, source_uuid, role, revision)
    WHERE reversal_of_id IS NULL;
CREATE INDEX idx_finance_entries_org_created ON finance_entries (organization_id, created_at);
CREATE INDEX idx_finance_entries_brand_created ON finance_entries (brand_id, created_at);
CREATE INDEX idx_finance_entries_cari_created ON finance_entries (cari_id, created_at, id)
    WHERE cari_id IS NOT NULL;
CREATE INDEX idx_finance_entries_account_created ON finance_entries (account_id, created_at, id)
    WHERE account_id IS NOT NULL;
-- VoidBySource reverses the rows of one source across organizations.
CREATE INDEX idx_finance_entries_source ON finance_entries (source_type, source_uuid)
    WHERE source_type IS NOT NULL;

-- Insert guard: brand and currency follow the organization (K7); account and
-- cari are kept in the same currency; a reversal mirrors exactly one
-- original row (same organization, direction, targets, currency and source;
-- negated amounts) and is never reversed itself.
CREATE FUNCTION finance_entries_check_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand    BIGINT;
    org_currency CHAR(3);
    acc_currency CHAR(3);
    orig         finance_entries%ROWTYPE;
BEGIN
    SELECT brand_id, currency INTO org_brand, org_currency
    FROM organizations WHERE id = NEW.organization_id;
    IF FOUND THEN
        IF org_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'finance_entries: brand % does not match organization % (brand %)',
                NEW.brand_id, NEW.organization_id, org_brand
                USING ERRCODE = 'check_violation';
        END IF;
        IF org_currency IS DISTINCT FROM NEW.currency THEN
            RAISE EXCEPTION 'finance_entries: currency % differs from organization currency %',
                NEW.currency, org_currency
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF NEW.account_id IS NOT NULL THEN
        SELECT currency INTO acc_currency FROM finance_accounts WHERE id = NEW.account_id;
        IF FOUND AND acc_currency IS DISTINCT FROM NEW.currency THEN
            RAISE EXCEPTION 'finance_entries: account % is kept in %, entry in %',
                NEW.account_id, acc_currency, NEW.currency
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    IF NEW.cari_id IS NOT NULL THEN
        SELECT currency INTO acc_currency FROM cari_accounts WHERE id = NEW.cari_id;
        IF FOUND AND acc_currency IS DISTINCT FROM NEW.currency THEN
            RAISE EXCEPTION 'finance_entries: cari % is kept in %, entry in %',
                NEW.cari_id, acc_currency, NEW.currency
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF NEW.reversal_of_id IS NOT NULL THEN
        SELECT * INTO orig FROM finance_entries WHERE id = NEW.reversal_of_id;
        IF NOT FOUND THEN
            RETURN NEW; -- the foreign key reports it
        END IF;
        IF orig.reversal_of_id IS NOT NULL THEN
            RAISE EXCEPTION 'finance_entries: entry % is a reversal and cannot be reversed', orig.id
                USING ERRCODE = 'check_violation';
        END IF;
        IF orig.organization_id <> NEW.organization_id
           OR orig.direction <> NEW.direction
           OR orig.account_id IS DISTINCT FROM NEW.account_id
           OR orig.cari_id IS DISTINCT FROM NEW.cari_id
           OR orig.currency <> NEW.currency
           OR orig.orig_currency <> NEW.orig_currency
           OR orig.amount <> -NEW.amount
           OR orig.orig_amount <> -NEW.orig_amount
           OR orig.source_type IS DISTINCT FROM NEW.source_type
           OR orig.source_uuid IS DISTINCT FROM NEW.source_uuid
           OR orig.role <> NEW.role
           OR orig.revision <> NEW.revision THEN
            RAISE EXCEPTION 'finance_entries: reversal does not mirror entry %', orig.id
                USING ERRCODE = 'check_violation',
                      HINT = 'A reversal copies the entry with negated amounts.';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_finance_entries_check_insert
    BEFORE INSERT ON finance_entries
    FOR EACH ROW
    EXECUTE FUNCTION finance_entries_check_insert();

-- Append-only: UPDATE, DELETE and TRUNCATE are refused. PG18 reports
-- restrict_violation as 23001; map it like 23503 (in use, 409).
CREATE FUNCTION finance_entries_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'finance_entries is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation',
              HINT = 'Correct an entry with a reversal (reversal_of_id).';
END;
$$;

CREATE TRIGGER trg_finance_entries_append_only
    BEFORE UPDATE OR DELETE ON finance_entries
    FOR EACH ROW
    EXECUTE FUNCTION finance_entries_append_only();

CREATE TRIGGER trg_finance_entries_no_truncate
    BEFORE TRUNCATE ON finance_entries
    FOR EACH STATEMENT
    EXECUTE FUNCTION finance_entries_append_only();

-- 4. Balance views (derived from the ledger; see the sign convention above).
CREATE VIEW cari_account_balances AS
SELECT c.id                AS cari_id,
       c.organization_id   AS organization_id,
       c.brand_id          AS brand_id,
       c.currency          AS currency,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'charge' THEN e.amount
                        WHEN 'payment' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'collection' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id)         AS entry_count,
       MAX(e.created_at)::timestamptz AS last_entry_at
FROM cari_accounts c
LEFT JOIN finance_entries e ON e.cari_id = c.id
GROUP BY c.id, c.organization_id, c.brand_id, c.currency;

CREATE VIEW finance_account_balances AS
SELECT a.id                AS account_id,
       a.organization_id   AS organization_id,
       a.brand_id          AS brand_id,
       a.currency          AS currency,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'collection' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'payment' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id)         AS entry_count,
       MAX(e.created_at)::timestamptz AS last_entry_at
FROM finance_accounts a
LEFT JOIN finance_entries e ON e.account_id = a.id
GROUP BY a.id, a.organization_id, a.brand_id, a.currency;

-- 5. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last, so sort_order continues after the current maximum).
-- TEC-99 decision 7: dealer roles read and dispute (K24); they no longer
-- write accounting. Distributors dispute what the center posts to them.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Dispute accounting entries', 'accounting.dispute', 'accounting',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Open a dispute on an entry posted by the parent organization (K24).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE slug IN ('dealer_owner', 'dealer_accounting'))
  AND permission_id = (SELECT id FROM permissions WHERE slug = 'accounting.write');

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'accounting.dispute', 'all'),
    ('distributor_owner', 'accounting.dispute', 'managed'),
    ('distributor_accounting', 'accounting.dispute', 'managed'),
    ('dealer_owner', 'accounting.dispute', 'managed'),
    ('dealer_accounting', 'accounting.dispute', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
