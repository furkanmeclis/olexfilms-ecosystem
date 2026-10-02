-- Reverts TEC-178. Data loss: every service, service item, image and
-- status log. Stock rows owned by a service keep owner_type 'service' with
-- an unchecked owner_id (as before 000050).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('services.complete', 'services.cancel')
);
DELETE FROM permissions WHERE slug IN ('services.complete', 'services.cancel');

DROP INDEX IF EXISTS idx_fixed_barcode_holdings_owner_service;
ALTER TABLE fixed_barcode_holdings
    DROP CONSTRAINT IF EXISTS chk_fixed_barcode_holdings_owner_service,
    DROP CONSTRAINT IF EXISTS fk_fixed_barcode_holdings_owner_service,
    DROP COLUMN IF EXISTS owner_service_id;

DROP INDEX IF EXISTS idx_unit_current_state_owner_service;
ALTER TABLE unit_current_state
    DROP CONSTRAINT IF EXISTS chk_unit_current_state_owner_service,
    DROP CONSTRAINT IF EXISTS fk_unit_current_state_owner_service,
    DROP COLUMN IF EXISTS owner_service_id;

DROP TABLE IF EXISTS service_status_logs;
DROP FUNCTION IF EXISTS service_status_logs_append_only();
DROP TABLE IF EXISTS service_images;
DROP TABLE IF EXISTS service_items;
DROP FUNCTION IF EXISTS service_items_check();
DROP TABLE IF EXISTS services;
DROP FUNCTION IF EXISTS services_check_row();
