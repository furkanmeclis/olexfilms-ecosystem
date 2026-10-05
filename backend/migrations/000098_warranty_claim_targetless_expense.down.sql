ALTER TABLE finance_entries DROP CONSTRAINT chk_finance_entries_targets;
ALTER TABLE finance_entries ADD CONSTRAINT chk_finance_entries_targets CHECK (
    CASE direction
        WHEN 'charge' THEN cari_id IS NOT NULL AND account_id IS NULL
        WHEN 'payment' THEN cari_id IS NOT NULL
        WHEN 'collection' THEN cari_id IS NOT NULL
        WHEN 'opening' THEN account_id IS NOT NULL AND cari_id IS NULL
        ELSE account_id IS NOT NULL OR cari_id IS NOT NULL OR source_type = 'staff_payment'
    END
);
