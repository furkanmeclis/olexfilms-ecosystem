DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'measurements.write');
DELETE FROM permissions WHERE slug = 'measurements.write';

DROP TABLE IF EXISTS measurement_devices;
DROP TABLE IF EXISTS measurement_results;
