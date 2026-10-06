-- TEC-172 (F1-07b): read models of the /v1/accounting endpoints. The caller
-- resolves the book (one organization inside the request scope) and passes
-- its id; every query is limited to that organization.

-- TEC-379 (DT-BE-8): the account grid is a full array (client-side
-- table); the sort follows docs/list-contract.md. type sorts by type, then
-- name (the old order); last_entry_at keeps unused accounts last.
-- name: ListFinanceAccountsWithBalance :many
SELECT a.*, b.balance, b.entry_count, b.last_entry_at
FROM finance_accounts a
JOIN finance_account_balances b ON b.account_id = a.id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR a.active = sqlc.narg(active)::bool)
  AND (COALESCE(cardinality(sqlc.narg(types)::text[]), 0) = 0 OR a.type = ANY (sqlc.narg(types)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR a.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR a.iban ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'name' THEN lower(a.name) WHEN 'type' THEN a.type || ' ' || lower(a.name) END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'name' THEN lower(a.name) WHEN 'type' THEN a.type || ' ' || lower(a.name) END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'balance' THEN b.balance END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'balance' THEN b.balance END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN a.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN a.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_entry_at' THEN b.last_entry_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_entry_at' THEN b.last_entry_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN a.id END DESC,
  a.id ASC;

-- name: GetFinanceAccountWithBalance :one
SELECT a.*, b.balance, b.entry_count, b.last_entry_at
FROM finance_accounts a
JOIN finance_account_balances b ON b.account_id = a.id
WHERE a.uuid = sqlc.arg(uuid) AND a.organization_id = sqlc.arg(organization_id);

-- TEC-379 (DT-BE-8): list contract (docs/list-contract.md). name is the
-- counterparty name (the old order); last_entry_at keeps caris without an
-- entry last; id is the tiebreak. kinds: center, distributor, dealer (the
-- counterparty organization type) or customer (a user counterparty).
-- name: ListCariAccountsWithBalance :many
SELECT c.*, b.balance, b.entry_count, b.last_entry_at,
       o.uuid AS counterparty_org_uuid, o.name AS counterparty_org_name, o.type AS counterparty_org_type,
       u.uuid AS counterparty_user_uuid,
       COALESCE(NULLIF(btrim(concat_ws(' ', u.name, u.surname)), ''), '')::text AS counterparty_user_name
FROM cari_accounts c
JOIN cari_account_balances b ON b.cari_id = c.id
LEFT JOIN organizations o ON o.id = c.counterparty_org_id
LEFT JOIN users u ON u.id = c.counterparty_user_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR c.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR concat_ws(' ', u.name, u.surname) ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
       OR (CASE WHEN c.counterparty_type = 'user' THEN 'customer' ELSE o.type END) = ANY (sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(balance_min)::numeric IS NULL OR b.balance >= sqlc.narg(balance_min)::numeric)
  AND (sqlc.narg(balance_max)::numeric IS NULL OR b.balance <= sqlc.narg(balance_max)::numeric)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN lower(COALESCE(o.name, NULLIF(btrim(concat_ws(' ', u.name, u.surname)), ''), '')) END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN lower(COALESCE(o.name, NULLIF(btrim(concat_ws(' ', u.name, u.surname)), ''), '')) END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'balance' THEN b.balance WHEN 'entry_count' THEN b.entry_count::numeric END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'balance' THEN b.balance WHEN 'entry_count' THEN b.entry_count::numeric END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN c.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_entry_at' THEN b.last_entry_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_entry_at' THEN b.last_entry_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN c.id END DESC,
  c.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCariAccountsWithBalance :one
SELECT COUNT(*)
FROM cari_accounts c
JOIN cari_account_balances b ON b.cari_id = c.id
LEFT JOIN organizations o ON o.id = c.counterparty_org_id
LEFT JOIN users u ON u.id = c.counterparty_user_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR c.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR concat_ws(' ', u.name, u.surname) ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
       OR (CASE WHEN c.counterparty_type = 'user' THEN 'customer' ELSE o.type END) = ANY (sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(balance_min)::numeric IS NULL OR b.balance >= sqlc.narg(balance_min)::numeric)
  AND (sqlc.narg(balance_max)::numeric IS NULL OR b.balance <= sqlc.narg(balance_max)::numeric);

-- name: GetCariAccountWithBalance :one
SELECT c.*, b.balance, b.entry_count, b.last_entry_at,
       o.uuid AS counterparty_org_uuid, o.name AS counterparty_org_name, o.type AS counterparty_org_type,
       u.uuid AS counterparty_user_uuid,
       COALESCE(NULLIF(btrim(concat_ws(' ', u.name, u.surname)), ''), '')::text AS counterparty_user_name
FROM cari_accounts c
JOIN cari_account_balances b ON b.cari_id = c.id
LEFT JOIN organizations o ON o.id = c.counterparty_org_id
LEFT JOIN users u ON u.id = c.counterparty_user_id
WHERE c.uuid = sqlc.arg(uuid) AND c.organization_id = sqlc.arg(organization_id);

-- SearchFinanceEntries is the filtered, paged ledger of one organization.
-- reversed_by_uuid is set when the row has been reversed (void).
-- TEC-379 (DT-BE-8): list contract (docs/list-contract.md): direction,
-- category and source_type are lists, amount_min / amount_max bound the
-- signed amount in the book currency, q matches the description, the
-- account name or the cari counterparty name; id is the tiebreak (an empty
-- sort_key orders by id only, as the single-entry lookup does).
-- name: SearchFinanceEntries :many
SELECT e.*,
       a.uuid AS account_uuid, a.name AS account_name,
       c.uuid AS cari_uuid,
       co.uuid AS counterparty_org_uuid, co.name AS counterparty_org_name,
       r.uuid AS reversed_by_uuid,
       o.uuid AS reversal_of_uuid
FROM finance_entries e
LEFT JOIN finance_accounts a ON a.id = e.account_id
LEFT JOIN cari_accounts c ON c.id = e.cari_id
LEFT JOIN organizations co ON co.id = c.counterparty_org_id
LEFT JOIN finance_entries r ON r.reversal_of_id = e.id
LEFT JOIN finance_entries o ON o.id = e.reversal_of_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(cari_id)::bigint IS NULL OR e.cari_id = sqlc.narg(cari_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(directions)::text[]), 0) = 0 OR e.direction = ANY (sqlc.narg(directions)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(categories)::text[]), 0) = 0 OR e.category = ANY (sqlc.narg(categories)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(source_types)::text[]), 0) = 0 OR e.source_type = ANY (sqlc.narg(source_types)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(amount_min)::numeric IS NULL OR e.amount >= sqlc.narg(amount_min)::numeric)
  AND (sqlc.narg(amount_max)::numeric IS NULL OR e.amount <= sqlc.narg(amount_max)::numeric)
  AND (sqlc.narg(q)::text IS NULL
       OR e.description ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (SELECT 1 FROM finance_accounts qa
                  WHERE qa.id = e.account_id AND qa.name ILIKE '%' || sqlc.narg(q)::text || '%')
       OR EXISTS (SELECT 1 FROM cari_accounts qc
                  LEFT JOIN organizations qo ON qo.id = qc.counterparty_org_id
                  LEFT JOIN users qu ON qu.id = qc.counterparty_user_id
                  WHERE qc.id = e.cari_id
                    AND (qo.name ILIKE '%' || sqlc.narg(q)::text || '%'
                         OR concat_ws(' ', qu.name, qu.surname) ILIKE '%' || sqlc.narg(q)::text || '%')))
  AND (sqlc.narg(entry_uuid)::uuid IS NULL OR e.uuid = sqlc.narg(entry_uuid)::uuid)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'direction' THEN e.direction WHEN 'category' THEN e.category END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'direction' THEN e.direction WHEN 'category' THEN e.category END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'amount' THEN e.amount END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'amount' THEN e.amount END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN e.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN e.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN e.id END DESC,
  e.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountSearchFinanceEntries :one
SELECT COUNT(*)
FROM finance_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(cari_id)::bigint IS NULL OR e.cari_id = sqlc.narg(cari_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(directions)::text[]), 0) = 0 OR e.direction = ANY (sqlc.narg(directions)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(categories)::text[]), 0) = 0 OR e.category = ANY (sqlc.narg(categories)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(source_types)::text[]), 0) = 0 OR e.source_type = ANY (sqlc.narg(source_types)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(amount_min)::numeric IS NULL OR e.amount >= sqlc.narg(amount_min)::numeric)
  AND (sqlc.narg(amount_max)::numeric IS NULL OR e.amount <= sqlc.narg(amount_max)::numeric)
  AND (sqlc.narg(q)::text IS NULL
       OR e.description ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (SELECT 1 FROM finance_accounts qa
                  WHERE qa.id = e.account_id AND qa.name ILIKE '%' || sqlc.narg(q)::text || '%')
       OR EXISTS (SELECT 1 FROM cari_accounts qc
                  LEFT JOIN organizations qo ON qo.id = qc.counterparty_org_id
                  LEFT JOIN users qu ON qu.id = qc.counterparty_user_id
                  WHERE qc.id = e.cari_id
                    AND (qo.name ILIKE '%' || sqlc.narg(q)::text || '%'
                         OR concat_ws(' ', qu.name, qu.surname) ILIKE '%' || sqlc.narg(q)::text || '%')));

-- name: GetFinanceEntryInOrgByUUID :one
SELECT * FROM finance_entries
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);
