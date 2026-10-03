DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions
                        WHERE slug IN ('integrations.glorian.view', 'integrations.glorian.manage'));
DELETE FROM permissions WHERE slug IN ('integrations.glorian.view', 'integrations.glorian.manage');

DROP TABLE IF EXISTS order_outbounds;
DROP FUNCTION IF EXISTS order_outbounds_check_order();
DROP TABLE IF EXISTS integration_external_parties;
DROP TABLE IF EXISTS integration_sync_runs;
DROP TABLE IF EXISTS connection_location_maps;
DROP TABLE IF EXISTS integration_connections;
