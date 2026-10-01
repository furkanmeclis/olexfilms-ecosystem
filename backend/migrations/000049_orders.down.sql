-- Reverts TEC-165. Data loss: every order, order line, assigned unit,
-- status history row, stock reservation and sibling transfer request.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'orders.read', 'orders.write', 'orders.approve', 'orders.ship',
        'orders.receive', 'orders.cancel', 'transfers.request', 'transfers.approve'
    )
);
DELETE FROM permissions WHERE slug IN (
    'orders.read', 'orders.write', 'orders.approve', 'orders.ship',
    'orders.receive', 'orders.cancel', 'transfers.request', 'transfers.approve'
);

DROP TABLE IF EXISTS stock_transfer_requests;
DROP FUNCTION IF EXISTS stock_transfer_requests_check_parties();
DROP TABLE IF EXISTS order_status_history;
DROP FUNCTION IF EXISTS order_status_history_append_only();
DROP TABLE IF EXISTS order_item_units;
DROP TABLE IF EXISTS stock_reservations;
DROP FUNCTION IF EXISTS order_unit_check_item();
DROP TABLE IF EXISTS order_items;
DROP FUNCTION IF EXISTS order_items_check_amount();
DROP TABLE IF EXISTS orders;
DROP FUNCTION IF EXISTS orders_check_parties();
DROP SEQUENCE IF EXISTS order_no_seq;
