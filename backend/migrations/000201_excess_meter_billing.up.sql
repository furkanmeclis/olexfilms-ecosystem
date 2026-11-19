-- TEC-1250 (K7, 19.11 kapsam ekleme): a whole roll (or box) that goes with a
-- meter line because the rest would be shorter than the minimum cut carries
-- extra meters. Whether the buyer pays them is an organization setting of
-- the seller: 'free' (default, the buyer pays the ordered meters) or
-- 'charge' (the buyer pays every meter that goes).
ALTER TABLE organizations
    ADD COLUMN excess_meter_billing TEXT NOT NULL DEFAULT 'free',
    ADD CONSTRAINT chk_organizations_excess_meter_billing CHECK (
        excess_meter_billing IN ('free', 'charge')
    );

-- The setting is frozen on the line when the extra meters are decided
-- (excess_billing); a later change of the setting never touches it. On a
-- 'charge' line meters is raised to the real meters of its pieces (what is
-- invoiced) and ordered_meters keeps what the buyer ordered; on a 'free'
-- line meters stays the ordered meters and ordered_meters is NULL.
ALTER TABLE order_items
    ADD COLUMN excess_billing TEXT NULL,
    ADD COLUMN ordered_meters NUMERIC(10,2) NULL,
    ADD CONSTRAINT chk_order_items_excess_billing CHECK (
        excess_billing IS NULL OR (meters IS NOT NULL AND excess_billing IN ('free', 'charge'))
    ),
    ADD CONSTRAINT chk_order_items_ordered_meters CHECK (
        ordered_meters IS NULL OR (
            excess_billing = 'charge' AND ordered_meters > 0 AND ordered_meters <= meters
        )
    );
