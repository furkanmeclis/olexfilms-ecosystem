-- Reverts TEC-472. Data loss: fleet profiles, dealer links, report records,
-- the vehicle -> fleet assignment, the fleet permissions with their grants
-- and every fleet organization (their members go with them; a fleet still
-- referenced elsewhere, e.g. by a cari account, makes the revert fail).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('fleets.read', 'fleets.manage', 'fleets.plan', 'fleet.portal.read'));
DELETE FROM permissions
WHERE slug IN ('fleets.read', 'fleets.manage', 'fleets.plan', 'fleet.portal.read');

DROP TABLE IF EXISTS fleet_reports;
DROP TRIGGER IF EXISTS trg_vehicles_check_fleet ON vehicles;
DROP INDEX IF EXISTS idx_vehicles_fleet;
ALTER TABLE vehicles DROP COLUMN IF EXISTS fleet_org_id;
DROP TABLE IF EXISTS fleet_dealer_links;
DROP FUNCTION IF EXISTS fleet_dealer_links_check_cari();
DROP TABLE IF EXISTS fleet_profiles;
DROP FUNCTION IF EXISTS fleet_check_org();

DROP TRIGGER IF EXISTS trg_organizations_check_fleet ON organizations;
DROP FUNCTION IF EXISTS organizations_check_fleet();
DELETE FROM organizations WHERE type = 'fleet';

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_type,
    DROP CONSTRAINT IF EXISTS chk_organizations_parent;
ALTER TABLE organizations
    ADD CONSTRAINT chk_organizations_type
        CHECK (type IN ('center', 'distributor', 'dealer')),
    ADD CONSTRAINT chk_organizations_parent
        CHECK ((type = 'center' AND parent_id IS NULL) OR (type <> 'center' AND parent_id IS NOT NULL));
