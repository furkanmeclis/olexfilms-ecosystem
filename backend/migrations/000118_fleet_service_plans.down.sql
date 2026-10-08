ALTER TABLE appointments
    DROP CONSTRAINT IF EXISTS fk_appointments_plan,
    DROP CONSTRAINT IF EXISTS chk_appointments_source,
    ADD CONSTRAINT chk_appointments_source CHECK (source IN ('panel', 'portal', 'assistant', 'lead')),
    DROP COLUMN IF EXISTS plan_id;

DROP TABLE IF EXISTS fleet_service_plans;
DROP FUNCTION IF EXISTS fleet_service_plans_check_row();
