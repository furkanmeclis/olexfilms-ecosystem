-- TEC-308 (F3-08d): subscription period accounting, expiry reminders, a
-- scheduled status for future starts and module bundle grants kept apart
-- from the manual module values.
--
--   * service_subscriptions.status gains 'scheduled': a subscription whose
--     starts_on is after the assignment day waits (modules closed, no
--     periods) until the daily job activates it on starts_on.
--   * module_flags: a module bundle subscription used to overwrite the
--     organization's own row (admin / distributor value) with a 'service'
--     row and delete it on close, losing the manual value. The service row
--     is now a separate grant next to the manual row: the org unique index
--     skips service rows and a second unique index keeps one service grant
--     per organization x module. The effective value is manual OR grant.
--   * service_subscription_reminders: one row per (subscription, days
--     before ends_on) so a 30 / 7 day expiry reminder is sent once.

-- 1. Scheduled status.
ALTER TABLE service_subscriptions DROP CONSTRAINT chk_service_subscriptions_status;
ALTER TABLE service_subscriptions ADD CONSTRAINT chk_service_subscriptions_status CHECK (
    status IN ('scheduled', 'active', 'cancel_requested', 'cancelled', 'expired'));

CREATE INDEX idx_service_subscriptions_scheduled ON service_subscriptions (starts_on)
    WHERE status = 'scheduled';

-- 2. Module bundle grants beside the manual value.
DROP INDEX uq_module_flags_org;
CREATE UNIQUE INDEX uq_module_flags_org ON module_flags (scope, organization_id, module_key)
    WHERE organization_id IS NOT NULL AND source <> 'service';
CREATE UNIQUE INDEX uq_module_flags_service ON module_flags (organization_id, module_key)
    WHERE source = 'service';
ALTER TABLE module_flags ADD CONSTRAINT chk_module_flags_service CHECK (
    source <> 'service' OR (scope = 'org' AND enabled));

-- 3. Expiry reminders sent.
CREATE TABLE service_subscription_reminders (
    id               BIGSERIAL    PRIMARY KEY,
    subscription_id  BIGINT       NOT NULL,
    organization_id  BIGINT       NOT NULL,
    brand_id         BIGINT       NOT NULL,
    days_before      SMALLINT     NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_subscription_reminders UNIQUE (subscription_id, days_before),
    CONSTRAINT fk_service_subscription_reminders_subscription
        FOREIGN KEY (subscription_id, organization_id, brand_id)
        REFERENCES service_subscriptions (id, organization_id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_subscription_reminders_days CHECK (days_before > 0)
);
