-- A charged line goes back to its ordered meters before the columns go.
UPDATE order_items
SET meters = ordered_meters,
    line_total = ROUND(unit_price * ordered_meters, 2)
WHERE ordered_meters IS NOT NULL;

ALTER TABLE order_items
    DROP CONSTRAINT chk_order_items_ordered_meters,
    DROP CONSTRAINT chk_order_items_excess_billing,
    DROP COLUMN ordered_meters,
    DROP COLUMN excess_billing;

ALTER TABLE organizations
    DROP CONSTRAINT chk_organizations_excess_meter_billing,
    DROP COLUMN excess_meter_billing;
