-- TEC-480: the certificate policy checks the staff member who performed the
-- service. Existing rows fall back to created_by_user_id in the use case.
ALTER TABLE services
    ADD COLUMN IF NOT EXISTS performed_by_user_id BIGINT NULL REFERENCES users (id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_services_performed_by
    ON services (brand_id, performed_by_user_id, created_at)
    WHERE performed_by_user_id IS NOT NULL;
