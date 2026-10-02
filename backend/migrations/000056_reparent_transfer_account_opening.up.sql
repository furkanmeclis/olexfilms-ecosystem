-- TEC-198 (F1-07h): K25 re-parenting cari transfer + cash/bank opening
-- balance.
--
--   * organization_parent_changes: one row per re-parenting (K25, platform
--     only). Its uuid is the change id: the ledger rows of the cari transfer
--     are sourced (cari_transfer, change uuid), so the idempotency key is
--     organization + old/new parent + change id.
--   * finance_entries direction 'opening': the opening balance of a
--     cash/bank account. Account only (no cari), positive, never income or
--     expense, so it moves the account balance and stays out of P&L.

-- 1. Re-parenting history.
CREATE TABLE organization_parent_changes (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    old_parent_id    BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    new_parent_id    BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    actor_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_organization_parent_changes_uuid UNIQUE (uuid),
    CONSTRAINT chk_organization_parent_changes_moved CHECK (old_parent_id <> new_parent_id),
    CONSTRAINT chk_organization_parent_changes_not_self CHECK (
        organization_id <> old_parent_id AND organization_id <> new_parent_id
    )
);

CREATE INDEX idx_organization_parent_changes_org
    ON organization_parent_changes (organization_id, created_at);

-- 2. Cash/bank opening balance direction.
ALTER TABLE finance_entries DROP CONSTRAINT chk_finance_entries_direction;
ALTER TABLE finance_entries ADD CONSTRAINT chk_finance_entries_direction CHECK (direction IN (
    'income', 'expense', 'charge', 'payment', 'collection', 'opening'
));

ALTER TABLE finance_entries DROP CONSTRAINT chk_finance_entries_targets;
ALTER TABLE finance_entries ADD CONSTRAINT chk_finance_entries_targets CHECK (
    CASE direction
        WHEN 'charge' THEN cari_id IS NOT NULL AND account_id IS NULL
        WHEN 'payment' THEN cari_id IS NOT NULL
        WHEN 'collection' THEN cari_id IS NOT NULL
        WHEN 'opening' THEN account_id IS NOT NULL AND cari_id IS NULL
        ELSE account_id IS NOT NULL OR cari_id IS NOT NULL
    END
);

-- Cash/bank balance: income + collection + opening - expense - payment.
CREATE OR REPLACE VIEW finance_account_balances AS
SELECT a.id                AS account_id,
       a.organization_id   AS organization_id,
       a.brand_id          AS brand_id,
       a.currency          AS currency,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'collection' THEN e.amount
                        WHEN 'opening' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'payment' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id)         AS entry_count,
       MAX(e.created_at)::timestamptz AS last_entry_at
FROM finance_accounts a
LEFT JOIN finance_entries e ON e.account_id = a.id
GROUP BY a.id, a.organization_id, a.brand_id, a.currency;
