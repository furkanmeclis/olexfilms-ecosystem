-- TEC-475 (F5-02d): fleet bulk service plans.
--
-- A plan is the transaction header for a dealer's appointment series of one
-- fleet. The appointments keep their ordinary customer/vehicle shape so the
-- existing reminders and intake flow continue to work.

CREATE TABLE fleet_service_plans (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    fleet_org_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    fleet_link_id       BIGINT        NOT NULL REFERENCES fleet_dealer_links (id) ON DELETE RESTRICT,
    title               TEXT          NOT NULL,
    service_type        VARCHAR(120)  NOT NULL,
    note                TEXT          NOT NULL DEFAULT '',
    start_date          DATE          NOT NULL,
    daily_vehicle_limit INTEGER       NOT NULL,
    preferred_times     JSONB         NOT NULL DEFAULT '[]'::jsonb,
    status              VARCHAR(16)   NOT NULL DEFAULT 'scheduled',
    idempotency_key     VARCHAR(128)  NULL,
    cancel_reason       TEXT          NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    cancelled_by_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at        TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_fleet_service_plans_uuid UNIQUE (uuid),
    CONSTRAINT uq_fleet_service_plans_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT uq_fleet_service_plans_idempotency UNIQUE (organization_id, idempotency_key),
    CONSTRAINT chk_fleet_service_plans_title CHECK (btrim(title) <> ''),
    CONSTRAINT chk_fleet_service_plans_service_type CHECK (btrim(service_type) <> ''),
    CONSTRAINT chk_fleet_service_plans_note CHECK (char_length(note) <= 20000),
    CONSTRAINT chk_fleet_service_plans_daily_limit CHECK (daily_vehicle_limit > 0),
    CONSTRAINT chk_fleet_service_plans_preferred_times CHECK (jsonb_typeof(preferred_times) = 'array'),
    CONSTRAINT chk_fleet_service_plans_status CHECK (status IN ('scheduled', 'cancelled')),
    CONSTRAINT chk_fleet_service_plans_cancel CHECK (
        (status = 'cancelled') = (cancelled_at IS NOT NULL)
        AND (status = 'cancelled' OR cancel_reason IS NULL)
        AND (status = 'cancelled' OR cancelled_by_user_id IS NULL)
    )
);

CREATE INDEX idx_fleet_service_plans_fleet ON fleet_service_plans (fleet_org_id, created_at DESC, id DESC);
CREATE INDEX idx_fleet_service_plans_org ON fleet_service_plans (organization_id, created_at DESC, id DESC);

CREATE TRIGGER trg_fleet_service_plans_set_updated_at
    BEFORE UPDATE ON fleet_service_plans
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_fleet_service_plans_check_fleet
    BEFORE INSERT OR UPDATE OF fleet_org_id, brand_id ON fleet_service_plans
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('fleet_org_id', 'fleet');

CREATE FUNCTION fleet_service_plans_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    link_fleet BIGINT;
    link_status TEXT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id
           OR NEW.fleet_org_id <> OLD.fleet_org_id
           OR NEW.fleet_link_id <> OLD.fleet_link_id THEN
            RAISE EXCEPTION 'fleet_service_plans: owner of plan % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    SELECT fleet_org_id, status INTO link_fleet, link_status
    FROM fleet_dealer_links WHERE id = NEW.fleet_link_id;
    IF FOUND THEN
        IF link_fleet IS DISTINCT FROM NEW.fleet_org_id OR link_status <> 'active' THEN
            RAISE EXCEPTION 'fleet_service_plans: link % is not active for fleet %',
                NEW.fleet_link_id, NEW.fleet_org_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_fleet_service_plans_check_row
    BEFORE INSERT OR UPDATE ON fleet_service_plans
    FOR EACH ROW
    EXECUTE FUNCTION fleet_service_plans_check_row();

ALTER TABLE appointments
    ADD COLUMN plan_id BIGINT NULL,
    DROP CONSTRAINT chk_appointments_source,
    ADD CONSTRAINT chk_appointments_source CHECK (source IN ('panel', 'portal', 'assistant', 'lead', 'fleet_plan')),
    ADD CONSTRAINT fk_appointments_plan FOREIGN KEY (plan_id, organization_id, brand_id)
        REFERENCES fleet_service_plans (id, organization_id, brand_id) ON DELETE RESTRICT;

CREATE INDEX idx_appointments_plan ON appointments (plan_id) WHERE plan_id IS NOT NULL;
