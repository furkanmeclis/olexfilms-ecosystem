DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('measurements.read', 'measurements.link', 'measurement_devices.manage')
);
DELETE FROM permissions WHERE slug IN ('measurements.read', 'measurements.link', 'measurement_devices.manage');

COMMENT ON COLUMN services.measurement_result_id IS NULL;
ALTER TABLE services
    DROP COLUMN IF EXISTS measurement_checked_at,
    DROP COLUMN IF EXISTS measurement_check_required;

DROP TABLE IF EXISTS service_measurements;
DROP FUNCTION IF EXISTS service_measurements_check_service_org();
DROP TABLE IF EXISTS measurement_tires;
DROP TABLE IF EXISTS measurement_values;

DROP INDEX IF EXISTS idx_measurement_results_unparsed;
DROP INDEX IF EXISTS idx_measurement_results_customer;
DROP INDEX IF EXISTS idx_measurement_results_device;

ALTER TABLE measurement_results
    DROP CONSTRAINT IF EXISTS fk_measurement_results_device_org,
    DROP CONSTRAINT IF EXISTS uq_measurement_results_id_org,
    DROP CONSTRAINT IF EXISTS chk_measurement_results_pdf_key,
    DROP COLUMN IF EXISTS pdf_key,
    DROP COLUMN IF EXISTS parsed_at,
    DROP COLUMN IF EXISTS body_type,
    DROP COLUMN IF EXISTS customer_user_id,
    DROP COLUMN IF EXISTS device_id,
    DROP COLUMN IF EXISTS measured_at;

ALTER TABLE measurement_devices
    DROP CONSTRAINT IF EXISTS uq_measurement_devices_id_org,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS model;
