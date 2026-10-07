DROP INDEX IF EXISTS idx_services_performed_by;

ALTER TABLE services
    DROP COLUMN IF EXISTS performed_by_user_id;
