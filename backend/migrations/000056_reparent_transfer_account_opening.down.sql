-- Reverts TEC-198. Data loss: every cash/bank opening balance row (and its
-- reversals) and the re-parenting history. The cari transfer rows stay
-- (finance_entries is append-only; they use the existing directions).
CREATE OR REPLACE VIEW finance_account_balances AS
SELECT a.id                AS account_id,
       a.organization_id   AS organization_id,
       a.brand_id          AS brand_id,
       a.currency          AS currency,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'collection' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'payment' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id)         AS entry_count,
       MAX(e.created_at)::timestamptz AS last_entry_at
FROM finance_accounts a
LEFT JOIN finance_entries e ON e.account_id = a.id
GROUP BY a.id, a.organization_id, a.brand_id, a.currency;

ALTER TABLE finance_entries DISABLE TRIGGER trg_finance_entries_append_only;
DELETE FROM finance_entries WHERE direction = 'opening' AND reversal_of_id IS NOT NULL;
DELETE FROM finance_entries WHERE direction = 'opening';
ALTER TABLE finance_entries ENABLE TRIGGER trg_finance_entries_append_only;

ALTER TABLE finance_entries DROP CONSTRAINT chk_finance_entries_targets;
ALTER TABLE finance_entries ADD CONSTRAINT chk_finance_entries_targets CHECK (
    CASE direction
        WHEN 'charge' THEN cari_id IS NOT NULL AND account_id IS NULL
        WHEN 'payment' THEN cari_id IS NOT NULL
        WHEN 'collection' THEN cari_id IS NOT NULL
        ELSE account_id IS NOT NULL OR cari_id IS NOT NULL
    END
);

ALTER TABLE finance_entries DROP CONSTRAINT chk_finance_entries_direction;
ALTER TABLE finance_entries ADD CONSTRAINT chk_finance_entries_direction CHECK (direction IN (
    'income', 'expense', 'charge', 'payment', 'collection'
));

DROP TABLE IF EXISTS organization_parent_changes;
