-- TEC-1070: accounting home and queue (legacy olexfilms-warehouse
-- "Muhasebe kuyruğu") and the billing details of a cari organization.
--
-- 1) Billing details live on the organization (one legal entity, one set
--    of invoice details), so an org merge keeps them on the merged
--    organization as history and needs no extra key. The seller's
--    accounting fills them from the cari card (accounting.write over a cari
--    whose counterparty is below its book).
-- 2) An accepted return (stock_transfer_requests kind = return, received)
--    waits in the receiver's accounting queue until it is marked processed
--    (credit note cut outside the app). Orders use the TEC-1030 invoice
--    marks, so they need no column here.

ALTER TABLE organizations
    ADD COLUMN billing_title   VARCHAR(255) NULL,
    ADD COLUMN tax_office      VARCHAR(120) NULL,
    ADD COLUMN tax_no          VARCHAR(32)  NULL,
    ADD COLUMN billing_address TEXT         NULL,
    ADD COLUMN billing_email   VARCHAR(255) NULL,
    ADD COLUMN iban            VARCHAR(34)  NULL,
    ADD COLUMN billing_updated_at TIMESTAMPTZ NULL,
    ADD COLUMN billing_updated_by_user_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT chk_organizations_iban CHECK (iban IS NULL OR iban ~ '^[A-Z]{2}[0-9]{2}[A-Z0-9]{10,30}$');

ALTER TABLE stock_transfer_requests
    ADD COLUMN accounting_processed_at          TIMESTAMPTZ NULL,
    ADD COLUMN accounting_processed_by_user_id  BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN accounting_note                  TEXT        NULL;

-- Queue read: received returns of a receiver not processed yet.
CREATE INDEX idx_stock_transfer_requests_return_queue
    ON stock_transfer_requests (to_org_id, received_at)
    WHERE kind = 'return' AND status = 'received';
