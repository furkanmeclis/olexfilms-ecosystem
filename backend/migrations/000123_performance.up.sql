-- TEC-490 (F5-05a): performance and targets schema.
--
--   * performance_metrics_monthly: projection of one organization x month
--     (period YYYY-MM) x metric. Rebuilt idempotently by the metric worker
--     (F5-05b); value with its numerator/denominator for rates. Metric keys
--     are the Go constants in internal/modules/performance/model. Money
--     metrics carry the organization currency.
--   * performance_targets: a target of a distributor or dealer, set by the
--     brand center or by an ancestor distributor (only for organizations
--     below it). Service count or order volume, per month, quarter or year;
--     contract_ref is the organization contract period (contract end date)
--     the target was agreed with.
--   * staff_targets: monthly individual targets inside a dealer.
--   * bonus_rules / bonus_accruals: dealer owner's target-based bonus rules
--     and their monthly accruals; a posted accrual points to the staff
--     payment (F3-07) it became. bonus_settings keeps the payment day of the
--     bonus (planned staff payment date).
--   * weak_dealer_rules: center (or distributor, for its subtree) rules.
--     Only center rules open tasks (tasks are a center module); distributor
--     rules notify.
--   * tasks.auto_rule_id / tasks.auto_period: one automatic task per subject
--     organization x rule x month.

-- 1. Monthly metric projection ------------------------------------------------
CREATE TABLE performance_metrics_monthly (
    id               BIGSERIAL     PRIMARY KEY,
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    period           CHAR(7)       NOT NULL,
    metric           VARCHAR(32)   NOT NULL,
    value            NUMERIC(18,4) NOT NULL,
    numerator        NUMERIC(18,4) NULL,
    denominator      NUMERIC(18,4) NULL,
    currency         CHAR(3)       NULL,
    computed_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_performance_metrics_monthly UNIQUE (organization_id, period, metric),
    CONSTRAINT chk_performance_metrics_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_performance_metrics_metric CHECK (metric IN (
        'services_count', 'warranty_start_rate', 'measurement_rate', 'review_avg',
        'stock_turnover', 'contract_days_left', 'cari_overdue_amount', 'cari_overdue_days',
        'certificate_coverage', 'lead_conversion_rate', 'waste_ratio', 'order_volume'
    )),
    CONSTRAINT chk_performance_metrics_currency CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_performance_metrics_money_currency CHECK (
        (metric IN ('order_volume', 'cari_overdue_amount')) = (currency IS NOT NULL)
    ),
    CONSTRAINT chk_performance_metrics_denominator CHECK (denominator IS NULL OR denominator >= 0)
);

CREATE INDEX idx_performance_metrics_brand_period ON performance_metrics_monthly (brand_id, period, metric);

-- 2. Targets --------------------------------------------------------------------
CREATE TABLE performance_targets (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    target_org_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    metric              VARCHAR(32)   NOT NULL,
    period_kind         VARCHAR(16)   NOT NULL,
    period_start        DATE          NOT NULL,
    period_end          DATE          GENERATED ALWAYS AS (
        (period_start + CASE period_kind
            WHEN 'monthly' THEN INTERVAL '1 month'
            WHEN 'quarterly' THEN INTERVAL '3 months'
            ELSE INTERVAL '1 year'
        END)::date
    ) STORED,
    value               NUMERIC(18,2) NOT NULL,
    currency            CHAR(3)       NULL,
    contract_ref        DATE          NULL,
    note                TEXT          NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_performance_targets_uuid UNIQUE (uuid),
    CONSTRAINT uq_performance_targets_period UNIQUE (target_org_id, metric, period_kind, period_start),
    CONSTRAINT chk_performance_targets_not_self CHECK (target_org_id <> organization_id),
    CONSTRAINT chk_performance_targets_metric CHECK (metric IN ('services_count', 'order_volume')),
    CONSTRAINT chk_performance_targets_kind CHECK (period_kind IN ('monthly', 'quarterly', 'yearly')),
    CONSTRAINT chk_performance_targets_start CHECK (
        EXTRACT(DAY FROM period_start) = 1
        AND (period_kind <> 'quarterly' OR EXTRACT(MONTH FROM period_start) IN (1, 4, 7, 10))
        AND (period_kind <> 'yearly' OR EXTRACT(MONTH FROM period_start) = 1)
    ),
    CONSTRAINT chk_performance_targets_value CHECK (value > 0),
    CONSTRAINT chk_performance_targets_currency CHECK (
        (metric = 'order_volume' AND currency ~ '^[A-Z]{3}$')
        OR (metric = 'services_count' AND currency IS NULL)
    ),
    CONSTRAINT chk_performance_targets_note CHECK (note IS NULL OR char_length(note) <= 2000)
);

CREATE INDEX idx_performance_targets_org ON performance_targets (organization_id, period_start DESC, id);
CREATE INDEX idx_performance_targets_brand_period ON performance_targets (brand_id, period_start, id);

CREATE TRIGGER trg_performance_targets_set_updated_at
    BEFORE UPDATE ON performance_targets
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Row guard shared by targets and weak dealer rules' owner: the defining
-- organization is the center of the brand or a distributor of the brand; a
-- target's subject is a distributor or dealer of the brand, and a
-- distributor defines only for organizations below it.
CREATE FUNCTION performance_targets_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type    TEXT;
    o_brand   BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND (
        NEW.organization_id <> OLD.organization_id OR NEW.brand_id <> OLD.brand_id
        OR NEW.target_org_id <> OLD.target_org_id OR NEW.uuid <> OLD.uuid
    ) THEN
        RAISE EXCEPTION 'performance_targets: identity of target % cannot change', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        RETURN NEW;
    END IF;

    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (o_type NOT IN ('center', 'distributor') OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'performance_targets: owner % must be the center or a distributor of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;

    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.target_org_id;
    IF FOUND AND (o_type NOT IN ('distributor', 'dealer') OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'performance_targets: target % must be a distributor or dealer of brand %',
            NEW.target_org_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;

    IF NOT EXISTS (
        WITH RECURSIVE up(id, parent_id) AS (
            SELECT o.id, o.parent_id FROM organizations o WHERE o.id = NEW.target_org_id
            UNION
            SELECT p.id, p.parent_id FROM organizations p JOIN up ON p.id = up.parent_id
        )
        SELECT 1 FROM up WHERE up.id = NEW.organization_id AND up.id <> NEW.target_org_id
    ) THEN
        RAISE EXCEPTION 'performance_targets: target % is not below owner %',
            NEW.target_org_id, NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_performance_targets_check_row
    BEFORE INSERT OR UPDATE ON performance_targets
    FOR EACH ROW
    EXECUTE FUNCTION performance_targets_check_row();

-- 3. Staff targets --------------------------------------------------------------
CREATE TABLE staff_targets (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    user_id             BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    period              CHAR(7)       NOT NULL,
    metric              VARCHAR(32)   NOT NULL,
    value               NUMERIC(18,2) NOT NULL,
    currency            CHAR(3)       NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_staff_targets_uuid UNIQUE (uuid),
    CONSTRAINT uq_staff_targets_user_period UNIQUE (organization_id, user_id, period, metric),
    CONSTRAINT fk_staff_targets_member FOREIGN KEY (organization_id, user_id)
        REFERENCES organization_members (organization_id, user_id) ON DELETE CASCADE,
    CONSTRAINT chk_staff_targets_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_staff_targets_metric CHECK (metric IN ('services_count', 'service_revenue')),
    CONSTRAINT chk_staff_targets_value CHECK (value > 0),
    CONSTRAINT chk_staff_targets_currency CHECK (
        (metric = 'service_revenue' AND currency ~ '^[A-Z]{3}$')
        OR (metric = 'services_count' AND currency IS NULL)
    )
);

CREATE INDEX idx_staff_targets_org_period ON staff_targets (organization_id, period, user_id);

CREATE TRIGGER trg_staff_targets_set_updated_at
    BEFORE UPDATE ON staff_targets
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 4. Bonus rules, settings and accruals ---------------------------------------
CREATE TABLE bonus_rules (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name                VARCHAR(200)  NOT NULL,
    metric              VARCHAR(32)   NOT NULL,
    threshold_pct       NUMERIC(7,2)  NOT NULL,
    kind                VARCHAR(24)   NOT NULL,
    amount              NUMERIC(18,2) NULL,
    percent             NUMERIC(7,4)  NULL,
    currency            CHAR(3)       NULL,
    active              BOOLEAN       NOT NULL DEFAULT true,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bonus_rules_uuid UNIQUE (uuid),
    CONSTRAINT uq_bonus_rules_id_org UNIQUE (id, organization_id),
    CONSTRAINT chk_bonus_rules_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_bonus_rules_metric CHECK (metric IN ('services_count', 'service_revenue')),
    CONSTRAINT chk_bonus_rules_threshold CHECK (threshold_pct > 0 AND threshold_pct <= 1000),
    CONSTRAINT chk_bonus_rules_kind CHECK (
        (kind = 'fixed' AND amount > 0 AND currency ~ '^[A-Z]{3}$' AND percent IS NULL)
        OR (kind = 'percent_of_revenue' AND percent > 0 AND percent <= 100 AND amount IS NULL AND currency IS NULL)
    )
);

CREATE INDEX idx_bonus_rules_org_active ON bonus_rules (organization_id, active, id);

CREATE TRIGGER trg_bonus_rules_set_updated_at
    BEFORE UPDATE ON bonus_rules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Payment day of the bonus (S20): the approved accrual becomes a planned
-- staff payment dated on this day of the month after the period.
CREATE TABLE bonus_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id         BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    payout_day       SMALLINT     NOT NULL DEFAULT 5,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_bonus_settings_payout_day CHECK (payout_day BETWEEN 1 AND 28)
);

CREATE TRIGGER trg_bonus_settings_set_updated_at
    BEFORE UPDATE ON bonus_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

ALTER TABLE staff_payments
    ADD CONSTRAINT uq_staff_payments_id_org UNIQUE (id, organization_id);

CREATE TABLE bonus_accruals (
    id                   BIGSERIAL     PRIMARY KEY,
    uuid                 UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id             BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    user_id              BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    period               CHAR(7)       NOT NULL,
    rule_id              BIGINT        NOT NULL,
    achievement_pct      NUMERIC(9,2)  NOT NULL,
    amount               NUMERIC(18,2) NOT NULL,
    currency             CHAR(3)       NOT NULL,
    status               VARCHAR(16)   NOT NULL DEFAULT 'calculated',
    staff_payment_id     BIGINT        NULL,
    approved_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    approved_at          TIMESTAMPTZ   NULL,
    cancelled_at         TIMESTAMPTZ   NULL,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bonus_accruals_uuid UNIQUE (uuid),
    CONSTRAINT fk_bonus_accruals_rule FOREIGN KEY (rule_id, organization_id)
        REFERENCES bonus_rules (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_bonus_accruals_staff_payment FOREIGN KEY (staff_payment_id, organization_id)
        REFERENCES staff_payments (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_bonus_accruals_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_bonus_accruals_achievement CHECK (achievement_pct >= 0),
    CONSTRAINT chk_bonus_accruals_amount CHECK (amount >= 0),
    CONSTRAINT chk_bonus_accruals_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_bonus_accruals_status CHECK (status IN ('calculated', 'approved', 'posted', 'cancelled')),
    CONSTRAINT chk_bonus_accruals_posted CHECK ((status = 'posted') = (staff_payment_id IS NOT NULL)),
    CONSTRAINT chk_bonus_accruals_approved CHECK (
        status NOT IN ('approved', 'posted') OR approved_at IS NOT NULL
    ),
    CONSTRAINT chk_bonus_accruals_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

-- One live accrual per user x rule x month; a cancelled one frees the slot.
CREATE UNIQUE INDEX uq_bonus_accruals_live ON bonus_accruals (user_id, rule_id, period)
    WHERE status <> 'cancelled';
CREATE INDEX idx_bonus_accruals_org_period ON bonus_accruals (organization_id, period, id);
CREATE INDEX idx_bonus_accruals_staff_payment ON bonus_accruals (staff_payment_id)
    WHERE staff_payment_id IS NOT NULL;

CREATE TRIGGER trg_bonus_accruals_set_updated_at
    BEFORE UPDATE ON bonus_accruals
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 5. Weak dealer rules -------------------------------------------------------
CREATE TABLE weak_dealer_rules (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    name                VARCHAR(200)  NOT NULL,
    -- A metric key, or target_achievement (achievement % of the active
    -- service count target).
    metric              VARCHAR(32)   NOT NULL,
    -- lt/lte/gt/gte compare the value with threshold; below_median_pct
    -- matches when the value is at least threshold % under the median of
    -- the rule's network (brand for the center, subtree for a distributor).
    operator            VARCHAR(24)   NOT NULL,
    threshold           NUMERIC(18,4) NOT NULL,
    create_task         BOOLEAN       NOT NULL DEFAULT false,
    notify              BOOLEAN       NOT NULL DEFAULT true,
    assignee_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    active              BOOLEAN       NOT NULL DEFAULT true,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_weak_dealer_rules_uuid UNIQUE (uuid),
    CONSTRAINT chk_weak_dealer_rules_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_weak_dealer_rules_metric CHECK (metric IN (
        'target_achievement',
        'services_count', 'warranty_start_rate', 'measurement_rate', 'review_avg',
        'stock_turnover', 'contract_days_left', 'cari_overdue_amount', 'cari_overdue_days',
        'certificate_coverage', 'lead_conversion_rate', 'waste_ratio', 'order_volume'
    )),
    CONSTRAINT chk_weak_dealer_rules_operator CHECK (operator IN ('lt', 'lte', 'gt', 'gte', 'below_median_pct')),
    CONSTRAINT chk_weak_dealer_rules_median_pct CHECK (
        operator <> 'below_median_pct' OR (threshold > 0 AND threshold <= 100)
    ),
    CONSTRAINT chk_weak_dealer_rules_action CHECK (create_task OR notify),
    CONSTRAINT chk_weak_dealer_rules_assignee CHECK (assignee_user_id IS NULL OR create_task)
);

CREATE INDEX idx_weak_dealer_rules_org_active ON weak_dealer_rules (organization_id, active, id);
CREATE INDEX idx_weak_dealer_rules_brand_active ON weak_dealer_rules (brand_id, active, id);

CREATE TRIGGER trg_weak_dealer_rules_set_updated_at
    BEFORE UPDATE ON weak_dealer_rules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Owner is the center or a distributor of the brand; only a center rule
-- opens tasks and its assignee is a member of that center.
CREATE FUNCTION weak_dealer_rules_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type   TEXT;
    o_brand  BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND (NEW.organization_id <> OLD.organization_id OR NEW.brand_id <> OLD.brand_id) THEN
        RAISE EXCEPTION 'weak_dealer_rules: owner of rule % cannot change', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (o_type NOT IN ('center', 'distributor') OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'weak_dealer_rules: owner % must be the center or a distributor of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.create_task AND o_type IS DISTINCT FROM 'center' THEN
        RAISE EXCEPTION 'weak_dealer_rules: only a center rule opens tasks'
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.assignee_user_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM organization_members m
        WHERE m.organization_id = NEW.organization_id AND m.user_id = NEW.assignee_user_id
    ) THEN
        RAISE EXCEPTION 'weak_dealer_rules: assignee % is not a member of %',
            NEW.assignee_user_id, NEW.organization_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_weak_dealer_rules_check_row
    BEFORE INSERT OR UPDATE ON weak_dealer_rules
    FOR EACH ROW
    EXECUTE FUNCTION weak_dealer_rules_check_row();

-- 6. Automatic tasks --------------------------------------------------------------
-- A rule that has opened tasks is deactivated rather than deleted
-- (ON DELETE RESTRICT keeps the task's origin).
ALTER TABLE tasks
    ADD COLUMN auto_rule_id BIGINT  NULL REFERENCES weak_dealer_rules (id) ON DELETE RESTRICT,
    ADD COLUMN auto_period  CHAR(7) NULL,
    ADD CONSTRAINT chk_tasks_auto_rule CHECK (
        (auto_rule_id IS NULL AND auto_period IS NULL)
        OR (source = 'auto' AND auto_rule_id IS NOT NULL
            AND auto_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
    ),
    ADD CONSTRAINT uq_tasks_auto UNIQUE (subject_org_id, auto_rule_id, auto_period);

-- Permissions. Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read performance', 'performance.read', 'performance',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read monthly performance metrics, rankings and target achievement in scope.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage performance targets', 'performance.targets.manage', 'performance',
       ARRAY['subtree', 'brand', 'all']::text[], false, false,
       'Set service count and order volume targets for the organizations below.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage staff targets', 'performance.staff_targets.manage', 'performance',
       ARRAY['managed', 'all']::text[], false, false,
       'Set monthly individual targets for the staff of the organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage staff bonuses', 'performance.bonus.manage', 'performance',
       ARRAY['managed', 'all']::text[], false, false,
       'Define target-based bonus rules and approve bonus accruals of the organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage weak dealer rules', 'performance.rules.manage', 'performance',
       ARRAY['subtree', 'brand', 'all']::text[], false, false,
       'Define weak dealer rules that open tasks or send notifications.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'performance.read', 'all'),
    ('super_admin', 'performance.targets.manage', 'all'),
    ('super_admin', 'performance.staff_targets.manage', 'all'),
    ('super_admin', 'performance.bonus.manage', 'all'),
    ('super_admin', 'performance.rules.manage', 'all'),
    ('center_staff', 'performance.read', 'brand'),
    ('center_staff', 'performance.targets.manage', 'brand'),
    ('center_staff', 'performance.rules.manage', 'brand'),
    ('distributor_owner', 'performance.read', 'subtree'),
    ('distributor_owner', 'performance.targets.manage', 'subtree'),
    ('distributor_owner', 'performance.rules.manage', 'subtree'),
    ('distributor_staff', 'performance.read', 'subtree'),
    ('dealer_owner', 'performance.read', 'managed'),
    ('dealer_owner', 'performance.staff_targets.manage', 'managed'),
    ('dealer_owner', 'performance.bonus.manage', 'managed'),
    ('dealer_accounting', 'performance.bonus.manage', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
