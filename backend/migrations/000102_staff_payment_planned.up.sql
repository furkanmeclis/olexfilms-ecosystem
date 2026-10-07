-- TEC-381 (F3-07j): a staff payment is booked on its payment day (paid_on).
-- A payment dated in the future is kept as 'planned' with no ledger row; the
-- worker-core job books it when paid_on arrives in the organization's time
-- zone and flips it to 'posted'. A planned payment can be edited or
-- cancelled ('cancelled', no ledger effect); a posted one is undone only by
-- a reversal (existing rule). Existing rows are all posted; their ledger
-- rows keep their dates.
ALTER TABLE staff_payments
    ADD COLUMN status       VARCHAR(16) NOT NULL DEFAULT 'posted',
    ADD COLUMN account_id   BIGINT      NULL,
    ADD COLUMN cancelled_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_staff_payments_status CHECK (status IN ('planned', 'posted', 'cancelled')),
    ADD CONSTRAINT chk_staff_payments_unposted_entry CHECK (status = 'posted' OR finance_entry_id IS NULL),
    ADD CONSTRAINT fk_staff_payments_account FOREIGN KEY (account_id, organization_id)
        REFERENCES finance_accounts (id, organization_id) ON DELETE RESTRICT;

-- The cash / bank account a planned payment will be paid from; backfilled
-- for the posted rows from their ledger row.
UPDATE staff_payments p
SET account_id = e.account_id
FROM finance_entries e
WHERE e.id = p.finance_entry_id AND e.organization_id = p.organization_id
  AND e.account_id IS NOT NULL;

-- A cancelled planned salary frees its period like a voided one.
DROP INDEX uq_staff_payments_salary_period;
CREATE UNIQUE INDEX uq_staff_payments_salary_period ON staff_payments (staff_id, period)
    WHERE type = 'salary' AND voided_at IS NULL AND status <> 'cancelled';

-- Due scan of the posting job.
CREATE INDEX idx_staff_payments_planned_due ON staff_payments (paid_on, id)
    WHERE status = 'planned';
