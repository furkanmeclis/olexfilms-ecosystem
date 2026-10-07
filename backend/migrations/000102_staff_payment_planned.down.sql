-- Planned and cancelled payments have no ledger row and cannot be told
-- apart from booked ones without the status column: they are removed.
DROP INDEX IF EXISTS idx_staff_payments_planned_due;
DELETE FROM staff_payments WHERE status IN ('planned', 'cancelled');
DROP INDEX uq_staff_payments_salary_period;
CREATE UNIQUE INDEX uq_staff_payments_salary_period ON staff_payments (staff_id, period)
    WHERE type = 'salary' AND voided_at IS NULL;
ALTER TABLE staff_payments
    DROP CONSTRAINT fk_staff_payments_account,
    DROP CONSTRAINT chk_staff_payments_unposted_entry,
    DROP CONSTRAINT chk_staff_payments_status,
    DROP COLUMN cancelled_at,
    DROP COLUMN account_id,
    DROP COLUMN status;
