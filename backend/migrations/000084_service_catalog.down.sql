-- Reverts TEC-305. Data loss: the service catalog, distributor price
-- overrides, every service subscription, its accounted periods and
-- cancellation requests.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'service_catalog.manage', 'service_catalog.read',
        'service_subscriptions.assign', 'service_subscriptions.read',
        'service_subscriptions.cancel_request', 'service_subscriptions.cancel_approve')
);
DELETE FROM permissions WHERE slug IN (
    'service_catalog.manage', 'service_catalog.read',
    'service_subscriptions.assign', 'service_subscriptions.read',
    'service_subscriptions.cancel_request', 'service_subscriptions.cancel_approve');

DROP TABLE IF EXISTS service_subscription_cancel_requests;
DROP FUNCTION IF EXISTS service_subscription_cancel_requests_check_row();
DROP TABLE IF EXISTS service_subscription_periods;
DROP TABLE IF EXISTS service_subscriptions;
DROP FUNCTION IF EXISTS service_subscriptions_check_row();
DROP TABLE IF EXISTS service_price_overrides;
DROP FUNCTION IF EXISTS service_price_overrides_check_row();
DROP TABLE IF EXISTS service_catalog_modules;
DROP TABLE IF EXISTS service_catalog_items;
DROP FUNCTION IF EXISTS service_catalog_items_check_row();
