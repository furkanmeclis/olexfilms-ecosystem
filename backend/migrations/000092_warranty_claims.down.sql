-- Reverts TEC-334. Data loss: warranty claims, their parts, photos and
-- timeline, and the services.warranty_claim_id links are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'warranty_claims.read', 'warranty_claims.write',
        'warranty_claims.review', 'warranty_claims.decide')
);
DELETE FROM permissions WHERE slug IN (
    'warranty_claims.read', 'warranty_claims.write',
    'warranty_claims.review', 'warranty_claims.decide');

DROP TRIGGER IF EXISTS trg_services_check_warranty_claim ON services;
DROP FUNCTION IF EXISTS services_check_warranty_claim();
DROP INDEX IF EXISTS idx_services_warranty_claim;
ALTER TABLE services DROP COLUMN IF EXISTS warranty_claim_id;

DROP TABLE IF EXISTS warranty_claim_events;
DROP FUNCTION IF EXISTS warranty_claim_events_append_only();
DROP TABLE IF EXISTS warranty_claim_photos;
DROP TABLE IF EXISTS warranty_claim_parts;
DROP FUNCTION IF EXISTS warranty_claim_parts_check_row();
DROP TABLE IF EXISTS warranty_claims;
DROP FUNCTION IF EXISTS warranty_claims_write_event();
DROP FUNCTION IF EXISTS warranty_claims_check_row();
