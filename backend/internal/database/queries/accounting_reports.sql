-- TEC-175 (F1-07e): cari statement and balance report. Read only; the caller
-- resolves the book (one organization inside the request scope) and passes
-- its id. Signs follow 000047: cari balance = income + charge + payment -
-- expense - collection (receivable positive); cash/bank balance = income +
-- collection + opening - expense - payment (opening: TEC-198, 000056).
-- Reversal rows carry negated amounts, so a plain sum nets them out.

-- GetCariStatementOpening is the cari balance before created_before (the
-- opening balance of a statement period).
-- name: GetCariStatementOpening :one
SELECT COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'charge' THEN e.amount
                        WHEN 'payment' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'collection' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance
FROM finance_entries e
WHERE e.cari_id = sqlc.arg(cari_id)
  AND e.organization_id = sqlc.arg(organization_id)
  AND e.created_at < sqlc.arg(created_before)::timestamptz;

-- ListCariStatementLines returns the period rows of a cari in ledger order
-- with their signed cari effect (signed_amount).
-- name: ListCariStatementLines :many
SELECT e.id, e.uuid, e.created_at, e.direction, e.category, e.description,
       e.source_type, e.source_uuid, e.orig_currency, e.orig_amount, e.currency,
       (CASE e.direction
            WHEN 'income' THEN e.amount
            WHEN 'charge' THEN e.amount
            WHEN 'payment' THEN e.amount
            WHEN 'expense' THEN -e.amount
            WHEN 'collection' THEN -e.amount
        END)::NUMERIC(18,2) AS signed_amount,
       ro.uuid AS reversal_of_uuid,
       (rv.id IS NOT NULL)::boolean AS reversed
FROM finance_entries e
LEFT JOIN finance_entries ro ON ro.id = e.reversal_of_id
LEFT JOIN finance_entries rv ON rv.reversal_of_id = e.id
WHERE e.cari_id = sqlc.arg(cari_id)
  AND e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
ORDER BY e.created_at, e.id;

-- ListCariBalancesAsOf is every cari of the book with its balance over the
-- rows written before created_to (NULL = all rows).
-- name: ListCariBalancesAsOf :many
SELECT c.uuid, c.counterparty_type, c.currency, c.active,
       o.uuid AS counterparty_org_uuid, o.name AS counterparty_org_name, o.type AS counterparty_org_type,
       u.uuid AS counterparty_user_uuid,
       COALESCE(NULLIF(btrim(concat_ws(' ', u.name, u.surname)), ''), '')::text AS counterparty_user_name,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'charge' THEN e.amount
                        WHEN 'payment' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'collection' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id) AS entry_count
FROM cari_accounts c
LEFT JOIN finance_entries e
       ON e.cari_id = c.id
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
LEFT JOIN organizations o ON o.id = c.counterparty_org_id
LEFT JOIN users u ON u.id = c.counterparty_user_id
WHERE c.organization_id = sqlc.arg(organization_id)
GROUP BY c.id, o.id, u.id
ORDER BY COALESCE(o.name, u.name, ''), c.id;

-- ListFinanceAccountBalancesAsOf is every cash/bank account of the book with
-- its balance over the rows written before created_to (NULL = all rows).
-- name: ListFinanceAccountBalancesAsOf :many
SELECT a.uuid, a.type, a.name, a.currency, a.active,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'collection' THEN e.amount
                        WHEN 'opening' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'payment' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id) AS entry_count
FROM finance_accounts a
LEFT JOIN finance_entries e
       ON e.account_id = a.id
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
WHERE a.organization_id = sqlc.arg(organization_id)
GROUP BY a.id
ORDER BY a.type, a.name, a.id;
