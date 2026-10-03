-- TEC-229 (K24 / TEC-223): a sale must not be reversed twice, once by a
-- dispute resolved with reversal and once by a received return.
--
-- stock_transfer_request_items gets the source it reverses:
--   * order_item_id: for a return line, the parent's order line the unit was
--     sold on (frozen at approval together with the price, the "latest order
--     line" rule of GetUnitLastOrderPrice). It ties the return line to the
--     order source (accounting source order + orders.uuid) that a dispute
--     may reverse.
--   * accounting_excluded: the line was received but not booked because the
--     sale of its order was already reversed by a dispute; no accounting
--     row is written for it (the transfer view shows it as a warning).
-- Existing priced return lines are backfilled with the same rule.

ALTER TABLE stock_transfer_request_items
    ADD COLUMN order_item_id BIGINT NULL REFERENCES order_items (id) ON DELETE RESTRICT,
    ADD COLUMN accounting_excluded BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_stock_transfer_request_items_order_item
    ON stock_transfer_request_items (order_item_id) WHERE order_item_id IS NOT NULL;

-- Disputes resolved with a reversal, looked up by source on return receipt.
CREATE INDEX idx_accounting_disputes_source_reversal
    ON accounting_disputes (source_type, source_uuid) WHERE status = 'resolved_reversal';

UPDATE stock_transfer_request_items i
SET order_item_id = (
    SELECT oi.id
    FROM order_item_units oiu
    JOIN order_items oi ON oi.id = oiu.order_item_id
    JOIN orders o ON o.id = oi.order_id
    WHERE oiu.unit_id = i.unit_id
      AND o.seller_org_id = r.to_org_id
      AND o.buyer_org_id = r.from_org_id
      AND o.currency::text = r.currency::text
      AND o.status NOT IN ('draft', 'cancelling', 'cancelled')
    ORDER BY oiu.assigned_at DESC, oiu.id DESC
    LIMIT 1
)
FROM stock_transfer_requests r
WHERE r.id = i.request_id
  AND r.kind = 'return'
  AND i.line_total IS NOT NULL;
