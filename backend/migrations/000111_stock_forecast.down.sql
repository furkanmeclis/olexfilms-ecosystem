-- Reverts TEC-483. Data loss: stock forecast snapshots, threshold overrides
-- and network demand forecast snapshots are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'stock_forecast.read',
        'stock_forecast.manage',
        'stock_forecast.network.read')
);
DELETE FROM permissions WHERE slug IN (
    'stock_forecast.read',
    'stock_forecast.manage',
    'stock_forecast.network.read'
);

DROP TABLE IF EXISTS network_demand_forecasts;
DROP TABLE IF EXISTS stock_forecast_thresholds;
DROP TABLE IF EXISTS stock_forecasts;
