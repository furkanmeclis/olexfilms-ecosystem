-- Data-loss: drops territories, exchange rates, plate formats and the
-- province/district references of organizations (city/district text stays).

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.slug IN ('platform.geo.write', 'platform.territories.read', 'platform.territories.write',
                 'platform.rates.read', 'platform.rates.write');

DELETE FROM permissions
WHERE slug IN ('platform.geo.write', 'platform.territories.read', 'platform.territories.write',
               'platform.rates.read', 'platform.rates.write');

DROP TABLE IF EXISTS exchange_rates;
DROP TABLE IF EXISTS currencies;
DROP TABLE IF EXISTS plate_formats;
DROP TABLE IF EXISTS territories;

DROP INDEX IF EXISTS idx_organizations_address;
ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_address_chain,
    DROP CONSTRAINT IF EXISTS fk_organizations_country,
    DROP COLUMN IF EXISTS district_id,
    DROP COLUMN IF EXISTS province_id;
-- country_id stays (TEC-83 column) as a plain nullable reference.

DROP TABLE IF EXISTS districts;
DROP TABLE IF EXISTS provinces;
DROP TABLE IF EXISTS countries;
