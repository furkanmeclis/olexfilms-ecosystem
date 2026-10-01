-- TEC-172 (F1-07b): read models of the /v1/accounting endpoints. The caller
-- resolves the book (one organization inside the request scope) and passes
-- its id; every query is limited to that organization.

-- name: ListFinanceAccountsWithBalance :many
SELECT a.*, b.balance, b.entry_count, b.last_entry_at
FROM finance_accounts a
JOIN finance_account_balances b ON b.account_id = a.id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR a.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(type)::text IS NULL OR a.type = sqlc.narg(type)::text)
ORDER BY a.type, a.name, a.id;

-- name: GetFinanceAccountWithBalance :one
SELECT a.*, b.balance, b.entry_count, b.last_entry_at
FROM finance_accounts a
JOIN finance_account_balances b ON b.account_id = a.id
WHERE a.uuid = sqlc.arg(uuid) AND a.organization_id = sqlc.arg(organization_id);

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
ORDER BY COALESCE(o.name, u.name, ''), c.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCariAccountsWithBalance :one
SELECT COUNT(*)
FROM cari_accounts c
LEFT JOIN organizations o ON o.id = c.counterparty_org_id
LEFT JOIN users u ON u.id = c.counterparty_user_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR c.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR concat_ws(' ', u.name, u.surname) ILIKE '%' || sqlc.narg(q)::text || '%');

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
  AND (sqlc.narg(entry_uuid)::uuid IS NULL OR e.uuid = sqlc.narg(entry_uuid)::uuid)
  AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(cari_id)::bigint IS NULL OR e.cari_id = sqlc.narg(cari_id)::bigint)
  AND (sqlc.narg(direction)::text IS NULL OR e.direction = sqlc.narg(direction)::text)
  AND (sqlc.narg(category)::text IS NULL OR e.category = sqlc.narg(category)::text)
  AND (sqlc.narg(source_type)::text IS NULL OR e.source_type = sqlc.narg(source_type)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountSearchFinanceEntries :one
SELECT COUNT(*)
FROM finance_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(cari_id)::bigint IS NULL OR e.cari_id = sqlc.narg(cari_id)::bigint)
  AND (sqlc.narg(direction)::text IS NULL OR e.direction = sqlc.narg(direction)::text)
  AND (sqlc.narg(category)::text IS NULL OR e.category = sqlc.narg(category)::text)
  AND (sqlc.narg(source_type)::text IS NULL OR e.source_type = sqlc.narg(source_type)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz);

-- name: GetFinanceEntryInOrgByUUID :one
SELECT * FROM finance_entries
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);
