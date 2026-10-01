DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('vehicle_catalog.read', 'vehicle_catalog.write'));
DELETE FROM permissions WHERE slug IN ('vehicle_catalog.read', 'vehicle_catalog.write');

DROP TABLE IF EXISTS car_models;
DROP TABLE IF EXISTS car_brands;
