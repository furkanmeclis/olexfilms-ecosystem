-- Reverts TEC-312. Data loss: leads, their event timeline, quotes, quote
-- lines, deliveries and reminders are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'quotes.read', 'quotes.write', 'leads.convert_org')
);
DELETE FROM permissions WHERE slug IN ('quotes.read', 'quotes.write', 'leads.convert_org');

-- Restore the pre-TEC-312 center_social lead grants seeded by 000029.
UPDATE role_permissions rp
SET scope = 'brand'
FROM roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug = 'center_social'
  AND p.slug IN ('leads.read', 'leads.write')
  AND rp.scope = 'all';

-- Drop grants that were introduced for roles which had no lead package in
-- 000029. Keep super_admin and center_social rows from the base catalog.
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND p.slug IN ('leads.read', 'leads.write')
  AND r.slug NOT IN ('super_admin', 'center_social');

DROP TABLE IF EXISTS quote_reminders;
DROP TABLE IF EXISTS quote_deliveries;
DROP TABLE IF EXISTS quote_lines;
DROP TABLE IF EXISTS quotes;
DROP TABLE IF EXISTS lead_events;
DROP FUNCTION IF EXISTS lead_events_append_only();
DROP TABLE IF EXISTS leads;
DROP FUNCTION IF EXISTS leads_write_event();
DROP FUNCTION IF EXISTS leads_check_row();
