-- TEC-1058 (RV-5.6): "sent plotter cut" mark of an order line. Information
-- only (no stock, price or flow rule reads it); the monthly plotter report
-- counts the shipped lines carrying it.
ALTER TABLE order_items
    ADD COLUMN plotter_cut BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_order_items_plotter_cut
    ON order_items (order_id)
    WHERE plotter_cut;
