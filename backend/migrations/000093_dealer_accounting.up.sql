-- TEC-341 (F3-07a): dealer accounting schema, permissions and sqlc.
-- Extends the F1-07 ledger (000047) without changing its semantics:
-- finance_entries stays append-only and sourced entries stay protected; the
-- new tables only point at ledger rows. APIs land in F3-07b..e, which also
-- gate them with RequireFeature(dealer_accounting).
--
--   * customer cari: cari_accounts.counterparty_type = 'user' (opened in
--     000047) gets a user lookup index and a guard that the customer is
--     served by the organization (customer_organizations).
--   * dealer_product_prices: the dealer's own sale price of a product.
--   * product_sales / product_sale_lines: quick product sale independent of
--     services; lines snapshot the purchase cost and may point at the stock
--     movement that took the unit out.
--   * suppliers / purchases / purchase_lines: the dealer's own purchases
--     with free-text lines; a purchase never moves stock (K12).
--   * staff_profiles / staff_payments: staff cards, salary, advance, bonus.
--   * services.income_entry_id / income_amount: per-service income record.

-- 1. Ledger target for organization-consistent foreign keys -----------------
-- (id is already unique; the pair lets child rows pin the organization.)
ALTER TABLE finance_entries
    ADD CONSTRAINT uq_finance_entries_id_org UNIQUE (id, organization_id);

-- 2. Customer cari ----------------------------------------------------------
-- uq_cari_accounts_org_counterparty_user (000047) already keeps one cari per
-- organization and customer; the FK to users exists. Add the user-side
-- lookup and require that the organization serves the customer. Existing
-- rows are not revalidated.
CREATE INDEX idx_cari_accounts_counterparty_user ON cari_accounts (counterparty_user_id)
    WHERE counterparty_user_id IS NOT NULL;

CREATE FUNCTION cari_accounts_check_customer() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.counterparty_user_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM customer_organizations co
                   WHERE co.user_id = NEW.counterparty_user_id
                     AND co.organization_id = NEW.organization_id) THEN
        RAISE EXCEPTION 'cari_accounts: user % is not a customer of organization %',
            NEW.counterparty_user_id, NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_cari_accounts_check_customer
    BEFORE INSERT OR UPDATE OF organization_id, counterparty_user_id ON cari_accounts
    FOR EACH ROW
    EXECUTE FUNCTION cari_accounts_check_customer();

-- 3. Dealer product prices --------------------------------------------------
CREATE TABLE dealer_product_prices (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id          BIGINT         NOT NULL,
    sale_price          NUMERIC(18,2)  NOT NULL,
    currency            CHAR(3)        NOT NULL,
    updated_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_dealer_product_prices_uuid UNIQUE (uuid),
    CONSTRAINT uq_dealer_product_prices_org_product UNIQUE (organization_id, product_id),
    CONSTRAINT fk_dealer_product_prices_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_dealer_product_prices_sale_price CHECK (sale_price >= 0),
    CONSTRAINT chk_dealer_product_prices_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX idx_dealer_product_prices_product ON dealer_product_prices (product_id);

CREATE TRIGGER trg_dealer_product_prices_set_updated_at
    BEFORE UPDATE ON dealer_product_prices
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_dealer_product_prices_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON dealer_product_prices
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. Product sales ----------------------------------------------------------
-- A quick sale outside services. customer_user_id is optional (walk-in); a
-- sale on cari needs the customer and its cari. finance_entry_id is the
-- income row the API posts through ledger.Post.
CREATE TABLE product_sales (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    customer_user_id    BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    payment_method      VARCHAR(16)    NOT NULL,
    cari_id             BIGINT         NULL,
    currency            CHAR(3)        NOT NULL,
    total               NUMERIC(18,2)  NOT NULL,
    sold_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    note                TEXT           NOT NULL DEFAULT '',
    finance_entry_id    BIGINT         NULL,
    created_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_sales_uuid UNIQUE (uuid),
    CONSTRAINT uq_product_sales_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_product_sales_cari FOREIGN KEY (cari_id, organization_id)
        REFERENCES cari_accounts (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_product_sales_finance_entry FOREIGN KEY (finance_entry_id, organization_id)
        REFERENCES finance_entries (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_product_sales_payment_method CHECK (payment_method IN ('cash', 'card', 'cari')),
    CONSTRAINT chk_product_sales_cari CHECK (
        (payment_method = 'cari') = (cari_id IS NOT NULL)
        AND (payment_method <> 'cari' OR customer_user_id IS NOT NULL)),
    CONSTRAINT chk_product_sales_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_product_sales_total CHECK (total >= 0),
    CONSTRAINT chk_product_sales_note CHECK (char_length(note) <= 5000)
);

CREATE INDEX idx_product_sales_org_sold_at ON product_sales (organization_id, sold_at DESC, id DESC);
CREATE INDEX idx_product_sales_customer ON product_sales (customer_user_id, sold_at)
    WHERE customer_user_id IS NOT NULL;
CREATE INDEX idx_product_sales_finance_entry ON product_sales (finance_entry_id)
    WHERE finance_entry_id IS NOT NULL;

CREATE TRIGGER trg_product_sales_set_updated_at
    BEFORE UPDATE ON product_sales
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_product_sales_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON product_sales
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Lines keep the purchase cost per unit at the time of sale (profit
-- analysis) and the stock movement that took the stock out, when any.
CREATE TABLE product_sale_lines (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    sale_id             BIGINT         NOT NULL,
    organization_id     BIGINT         NOT NULL,
    brand_id            BIGINT         NOT NULL,
    product_id          BIGINT         NOT NULL,
    unit_id             BIGINT         NULL,
    quantity            NUMERIC(10,2)  NOT NULL,
    unit_price          NUMERIC(18,2)  NOT NULL,
    line_total          NUMERIC(18,2)  NOT NULL,
    purchase_unit_cost  NUMERIC(18,2)  NULL,
    stock_movement_id   BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    sort_order          INT            NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_sale_lines_uuid UNIQUE (uuid),
    CONSTRAINT fk_product_sale_lines_sale FOREIGN KEY (sale_id, organization_id, brand_id)
        REFERENCES product_sales (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT fk_product_sale_lines_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_product_sale_lines_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_product_sale_lines_quantity CHECK (quantity > 0),
    CONSTRAINT chk_product_sale_lines_unit_price CHECK (unit_price >= 0),
    CONSTRAINT chk_product_sale_lines_line_total CHECK (line_total >= 0),
    CONSTRAINT chk_product_sale_lines_purchase_cost CHECK (purchase_unit_cost IS NULL OR purchase_unit_cost >= 0)
);

CREATE INDEX idx_product_sale_lines_sale ON product_sale_lines (sale_id, sort_order, id);
CREATE INDEX idx_product_sale_lines_product ON product_sale_lines (organization_id, product_id);
CREATE INDEX idx_product_sale_lines_stock_movement ON product_sale_lines (stock_movement_id)
    WHERE stock_movement_id IS NOT NULL;

-- 5. Suppliers --------------------------------------------------------------
CREATE TABLE suppliers (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name             VARCHAR(200)  NOT NULL,
    tax_no           VARCHAR(32)   NULL,
    phone_e164       VARCHAR(16)   NULL,
    email            VARCHAR(255)  NULL,
    note             TEXT          NOT NULL DEFAULT '',
    active           BOOLEAN       NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_suppliers_uuid UNIQUE (uuid),
    CONSTRAINT uq_suppliers_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_suppliers_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_suppliers_tax_no CHECK (tax_no IS NULL OR btrim(tax_no) <> ''),
    CONSTRAINT chk_suppliers_phone_e164 CHECK (
        phone_e164 IS NULL OR phone_e164 ~ '^\+[1-9][0-9]{7,14}$'),
    CONSTRAINT chk_suppliers_email CHECK (email IS NULL OR email ~ '^[^@\s]+@[^@\s]+$'),
    CONSTRAINT chk_suppliers_note CHECK (char_length(note) <= 5000)
);

CREATE INDEX idx_suppliers_org_name ON suppliers (organization_id, active, name);

CREATE TRIGGER trg_suppliers_set_updated_at
    BEFORE UPDATE ON suppliers
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_suppliers_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON suppliers
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 6. Purchases (no stock movement, K12) -------------------------------------
CREATE TABLE purchases (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    supplier_id         BIGINT         NOT NULL,
    purchased_on        DATE           NOT NULL,
    currency            CHAR(3)        NOT NULL,
    amount              NUMERIC(18,2)  NOT NULL,
    payment_method      VARCHAR(16)    NOT NULL,
    note                TEXT           NOT NULL DEFAULT '',
    finance_entry_id    BIGINT         NULL,
    created_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_purchases_uuid UNIQUE (uuid),
    CONSTRAINT uq_purchases_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT fk_purchases_supplier FOREIGN KEY (supplier_id, organization_id, brand_id)
        REFERENCES suppliers (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_purchases_finance_entry FOREIGN KEY (finance_entry_id, organization_id)
        REFERENCES finance_entries (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_purchases_payment_method CHECK (payment_method IN ('cash', 'card', 'bank_transfer', 'cari')),
    CONSTRAINT chk_purchases_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_purchases_amount CHECK (amount > 0),
    CONSTRAINT chk_purchases_note CHECK (char_length(note) <= 5000)
);

CREATE INDEX idx_purchases_org_date ON purchases (organization_id, purchased_on DESC, id DESC);
CREATE INDEX idx_purchases_supplier ON purchases (supplier_id, purchased_on DESC);
CREATE INDEX idx_purchases_finance_entry ON purchases (finance_entry_id)
    WHERE finance_entry_id IS NOT NULL;

CREATE TRIGGER trg_purchases_set_updated_at
    BEFORE UPDATE ON purchases
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_purchases_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON purchases
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE TABLE purchase_lines (
    id               BIGSERIAL      PRIMARY KEY,
    purchase_id      BIGINT         NOT NULL,
    organization_id  BIGINT         NOT NULL,
    brand_id         BIGINT         NOT NULL,
    description      TEXT           NOT NULL,
    quantity         NUMERIC(10,2)  NOT NULL DEFAULT 1,
    unit_price       NUMERIC(18,2)  NOT NULL,
    line_total       NUMERIC(18,2)  NOT NULL,
    sort_order       INT            NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_purchase_lines_purchase FOREIGN KEY (purchase_id, organization_id, brand_id)
        REFERENCES purchases (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_purchase_lines_description CHECK (
        btrim(description) <> '' AND char_length(description) <= 1000),
    CONSTRAINT chk_purchase_lines_quantity CHECK (quantity > 0),
    CONSTRAINT chk_purchase_lines_unit_price CHECK (unit_price >= 0),
    CONSTRAINT chk_purchase_lines_line_total CHECK (line_total >= 0)
);

CREATE INDEX idx_purchase_lines_purchase ON purchase_lines (purchase_id, sort_order, id);

-- 7. Staff ------------------------------------------------------------------
-- A staff card may exist without a panel user (user_id NULL).
CREATE TABLE staff_profiles (
    id               BIGSERIAL      PRIMARY KEY,
    uuid             UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    user_id          BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    name             VARCHAR(200)   NOT NULL,
    title            VARCHAR(100)   NULL,
    hired_on         DATE           NULL,
    monthly_salary   NUMERIC(18,2)  NULL,
    currency         CHAR(3)        NOT NULL,
    active           BOOLEAN        NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_staff_profiles_uuid UNIQUE (uuid),
    CONSTRAINT uq_staff_profiles_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_staff_profiles_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_staff_profiles_title CHECK (title IS NULL OR btrim(title) <> ''),
    CONSTRAINT chk_staff_profiles_salary CHECK (monthly_salary IS NULL OR monthly_salary >= 0),
    CONSTRAINT chk_staff_profiles_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE UNIQUE INDEX uq_staff_profiles_org_user ON staff_profiles (organization_id, user_id)
    WHERE user_id IS NOT NULL;
CREATE INDEX idx_staff_profiles_org_active ON staff_profiles (organization_id, active, name);

CREATE TRIGGER trg_staff_profiles_set_updated_at
    BEFORE UPDATE ON staff_profiles
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_staff_profiles_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON staff_profiles
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- One salary per staff and period (YYYY-MM); advances and bonuses repeat.
-- A voided payment (its ledger row reversed) frees the salary period.
CREATE TABLE staff_payments (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL,
    brand_id            BIGINT         NOT NULL,
    staff_id            BIGINT         NOT NULL,
    type                VARCHAR(16)    NOT NULL,
    period              CHAR(7)        NOT NULL,
    amount              NUMERIC(18,2)  NOT NULL,
    currency            CHAR(3)        NOT NULL,
    paid_on             DATE           NOT NULL DEFAULT CURRENT_DATE,
    description         TEXT           NULL,
    finance_entry_id    BIGINT         NULL,
    created_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    voided_at           TIMESTAMPTZ    NULL,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_staff_payments_uuid UNIQUE (uuid),
    CONSTRAINT fk_staff_payments_staff FOREIGN KEY (staff_id, organization_id, brand_id)
        REFERENCES staff_profiles (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_staff_payments_finance_entry FOREIGN KEY (finance_entry_id, organization_id)
        REFERENCES finance_entries (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_staff_payments_type CHECK (type IN ('salary', 'advance', 'bonus')),
    CONSTRAINT chk_staff_payments_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_staff_payments_amount CHECK (amount > 0),
    CONSTRAINT chk_staff_payments_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_staff_payments_description CHECK (description IS NULL OR char_length(description) <= 2000)
);

CREATE UNIQUE INDEX uq_staff_payments_salary_period ON staff_payments (staff_id, period)
    WHERE type = 'salary' AND voided_at IS NULL;
CREATE INDEX idx_staff_payments_org_period ON staff_payments (organization_id, period, staff_id);
CREATE INDEX idx_staff_payments_staff ON staff_payments (staff_id, paid_on DESC, id DESC);
CREATE INDEX idx_staff_payments_finance_entry ON staff_payments (finance_entry_id)
    WHERE finance_entry_id IS NOT NULL;

CREATE TRIGGER trg_staff_payments_set_updated_at
    BEFORE UPDATE ON staff_payments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_staff_payments_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON staff_payments
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 8. Per-service income record ----------------------------------------------
ALTER TABLE services
    ADD COLUMN income_entry_id BIGINT NULL,
    ADD COLUMN income_amount   NUMERIC(18,2) NULL,
    ADD CONSTRAINT fk_services_income_entry FOREIGN KEY (income_entry_id, organization_id)
        REFERENCES finance_entries (id, organization_id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_services_income_amount CHECK (income_amount IS NULL OR income_amount >= 0);

CREATE INDEX idx_services_income_entry ON services (income_entry_id) WHERE income_entry_id IS NOT NULL;

-- 9. Permissions ------------------------------------------------------------
-- Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write dealer product prices', 'dealer_pricing.write', 'dealer_accounting',
       ARRAY['managed', 'all']::text[], false, false,
       'Set the dealer''s own sale price of products.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write product sales', 'product_sales.write', 'dealer_accounting',
       ARRAY['managed', 'all']::text[], false, false,
       'Record quick product sales outside services.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage suppliers', 'suppliers.manage', 'dealer_accounting',
       ARRAY['managed', 'all']::text[], false, false,
       'Create and update the organization''s suppliers.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write purchases', 'purchases.write', 'dealer_accounting',
       ARRAY['managed', 'all']::text[], false, false,
       'Record purchases from suppliers (no stock movement).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage staff', 'staff.manage', 'dealer_accounting',
       ARRAY['managed', 'all']::text[], false, false,
       'Manage staff cards and salaries of the organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write staff payments', 'staff_payments.write', 'dealer_accounting',
       ARRAY['managed', 'all']::text[], false, false,
       'Record salary, advance and bonus payments to staff.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'dealer_pricing.write', 'all'),
    ('super_admin', 'product_sales.write', 'all'),
    ('super_admin', 'suppliers.manage', 'all'),
    ('super_admin', 'purchases.write', 'all'),
    ('super_admin', 'staff.manage', 'all'),
    ('super_admin', 'staff_payments.write', 'all'),
    ('dealer_owner', 'accounting.write', 'managed'),
    ('dealer_owner', 'dealer_pricing.write', 'managed'),
    ('dealer_owner', 'product_sales.write', 'managed'),
    ('dealer_owner', 'suppliers.manage', 'managed'),
    ('dealer_owner', 'purchases.write', 'managed'),
    ('dealer_owner', 'staff.manage', 'managed'),
    ('dealer_owner', 'staff_payments.write', 'managed'),
    ('dealer_accounting', 'accounting.write', 'managed'),
    ('dealer_accounting', 'dealer_pricing.write', 'managed'),
    ('dealer_accounting', 'product_sales.write', 'managed'),
    ('dealer_accounting', 'suppliers.manage', 'managed'),
    ('dealer_accounting', 'purchases.write', 'managed'),
    ('dealer_accounting', 'staff.manage', 'managed'),
    ('dealer_accounting', 'staff_payments.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
