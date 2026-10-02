-- Reverts TEC-185. Data loss: every warranty, vehicle transfer and the
-- organizations' Google Business links.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('warranties.read', 'warranties.void', 'vehicles.transfer')
);
DELETE FROM permissions WHERE slug IN ('warranties.read', 'warranties.void', 'vehicles.transfer');

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_google_business_url,
    DROP COLUMN IF EXISTS google_business_url;

DROP TABLE IF EXISTS vehicle_transfers;
DROP FUNCTION IF EXISTS vehicle_transfers_check_row();

DROP TABLE IF EXISTS warranties;
DROP FUNCTION IF EXISTS warranties_check_row();

ALTER TABLE service_items DROP CONSTRAINT IF EXISTS uq_service_items_warranty_ref;
