-- TEC-337 (F3-06d): the warranty cost of a closed claim is the center's own
-- P&L expense (category warranty_cost, source_type warranty_claim). It has no
-- counterparty and is not paid from a cash/bank account, so like the payroll
-- rows of 000097 it is a targetless sourced row. Manual entries still validate
-- a target in the accounting use case.
ALTER TABLE finance_entries DROP CONSTRAINT chk_finance_entries_targets;
ALTER TABLE finance_entries ADD CONSTRAINT chk_finance_entries_targets CHECK (
    CASE direction
        WHEN 'charge' THEN cari_id IS NOT NULL AND account_id IS NULL
        WHEN 'payment' THEN cari_id IS NOT NULL
        WHEN 'collection' THEN cari_id IS NOT NULL
        WHEN 'opening' THEN account_id IS NOT NULL AND cari_id IS NULL
        ELSE account_id IS NOT NULL OR cari_id IS NOT NULL
             OR source_type IN ('staff_payment', 'warranty_claim')
    END
);
