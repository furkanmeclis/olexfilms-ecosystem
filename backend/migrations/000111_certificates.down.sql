-- Reverts TEC-479. Data loss: certificate types, uploaded certificate rows
-- and service certificate warning decisions are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'certificate_types.manage', 'certificates.read', 'certificates.write',
        'certificates.verify', 'certificates.approve_service')
);
DELETE FROM permissions WHERE slug IN (
    'certificate_types.manage', 'certificates.read', 'certificates.write',
    'certificates.verify', 'certificates.approve_service');

DROP TABLE IF EXISTS service_certificate_warnings;
DROP TABLE IF EXISTS certificates;
DROP FUNCTION IF EXISTS certificates_check_row();
DROP TABLE IF EXISTS certificate_type_products;
DROP TABLE IF EXISTS certificate_type_categories;
DROP TABLE IF EXISTS certificate_types;
