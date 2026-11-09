-- TEC-1085: returns opened by the parent on behalf of a child and linked to
-- an order.
--
-- * order_id: the order (parent -> child sale) the return refers to. Only a
--   return carries it; the use case checks that the order was sold by the
--   receiver (to_org_id) to the giver (from_org_id). orders is rewritten by
--   an organization merge (TEC-1003), so the FK is deferred.
-- * opened_by_parent: the parent (to_org_id = approver_org_id) opened the
--   return for the child. Such a return starts approved and the parent may
--   ship and receive it itself (the child need not have a member).
ALTER TABLE stock_transfer_requests
    ADD COLUMN order_id BIGINT NULL,
    ADD COLUMN opened_by_parent BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT fk_stock_transfer_requests_order FOREIGN KEY (order_id)
        REFERENCES orders (id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT chk_stock_transfer_requests_order_return CHECK (order_id IS NULL OR kind = 'return'),
    ADD CONSTRAINT chk_stock_transfer_requests_opened_by_parent CHECK (NOT opened_by_parent OR kind = 'return');

CREATE INDEX idx_stock_transfer_requests_order ON stock_transfer_requests (order_id)
    WHERE order_id IS NOT NULL;
