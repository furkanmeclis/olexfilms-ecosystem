-- TEC-1228 (migration 000191; Linear 000186 ayırmıştı, main ilerlediği için taşındı): the warehouse work queue and the shipment details of an order.
--
-- * priority / due_date: the buyer sets them when it opens the order (and
--   while it is a draft or submitted), the seller side until it ships. The
--   warehouse lists sort urgent first, then by due date (sort=queue).
-- * picker_user_id / claimed_at: the warehouse user who took the job
--   ("işi üstlen"). Users are global (not rewritten by an organization
--   merge), so a plain FK is enough.
-- * package_count / desi / weight_kg: entered at shipping and printed on the
--   shipping note (no prices).
ALTER TABLE orders
    ADD COLUMN priority       TEXT          NOT NULL DEFAULT 'normal',
    ADD COLUMN due_date       DATE          NULL,
    ADD COLUMN picker_user_id BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN claimed_at     TIMESTAMPTZ   NULL,
    ADD COLUMN package_count  INTEGER       NULL,
    ADD COLUMN desi           NUMERIC(10,2) NULL,
    ADD COLUMN weight_kg      NUMERIC(10,2) NULL,
    ADD CONSTRAINT chk_orders_priority CHECK (priority IN ('normal', 'urgent')),
    ADD CONSTRAINT chk_orders_package_count CHECK (package_count IS NULL OR package_count > 0),
    ADD CONSTRAINT chk_orders_desi CHECK (desi IS NULL OR desi >= 0),
    ADD CONSTRAINT chk_orders_weight_kg CHECK (weight_kg IS NULL OR weight_kg >= 0);

-- The open warehouse queue of a seller: urgent first, then the due date.
CREATE INDEX idx_orders_seller_queue ON orders (organization_id, priority, due_date)
    WHERE status IN ('approved', 'processing', 'preparing', 'ready');

CREATE INDEX idx_orders_picker ON orders (picker_user_id)
    WHERE picker_user_id IS NOT NULL;
