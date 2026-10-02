-- TEC-171: accounting primitives for the finance/cari use cases (TEC-99b)
-- and the source API (TEC-99c). finance_entries is append-only: corrections
-- are reversal rows. Organization scope is applied by the caller.

-- ---------------------------------------------------------------------------
-- Cash and bank accounts.

-- name: CreateFinanceAccount :one
INSERT INTO finance_accounts (organization_id, brand_id, type, name, currency, iban, active)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(type), sqlc.arg(name),
    sqlc.arg(currency), sqlc.narg(iban), sqlc.arg(active)
)
RETURNING *;

-- name: GetFinanceAccount :one
SELECT * FROM finance_accounts
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: GetFinanceAccountByUUID :one
SELECT * FROM finance_accounts WHERE uuid = sqlc.arg(uuid);

-- name: ListFinanceAccounts :many
SELECT * FROM finance_accounts
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
ORDER BY type, name, id;

-- name: UpdateFinanceAccount :one
UPDATE finance_accounts
SET name = sqlc.arg(name),
    iban = sqlc.narg(iban),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Cari accounts.

-- name: CreateCariAccount :one
INSERT INTO cari_accounts (
    organization_id, brand_id, counterparty_type, counterparty_org_id, counterparty_user_id, currency
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(counterparty_type),
    sqlc.narg(counterparty_org_id), sqlc.narg(counterparty_user_id), sqlc.arg(currency)
)
RETURNING *;

-- CreateCariForOrgIfMissing opens the cari of organization_id with a
-- counterparty organization. When it already exists no row is returned
-- (pgx.ErrNoRows) and the caller reads it with GetCariAccountByCounterpartyOrg.
-- name: CreateCariForOrgIfMissing :one
INSERT INTO cari_accounts (organization_id, brand_id, counterparty_type, counterparty_org_id, currency)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), 'organization',
    sqlc.arg(counterparty_org_id), sqlc.arg(currency)
)
ON CONFLICT (organization_id, counterparty_org_id) WHERE counterparty_org_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCariAccount :one
SELECT * FROM cari_accounts
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: GetCariAccountByUUID :one
SELECT * FROM cari_accounts WHERE uuid = sqlc.arg(uuid);

-- name: GetCariAccountByCounterpartyOrg :one
SELECT * FROM cari_accounts
WHERE organization_id = sqlc.arg(organization_id)
  AND counterparty_org_id = sqlc.arg(counterparty_org_id);

-- name: ListCariAccounts :many
SELECT * FROM cari_accounts
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
ORDER BY created_at, id;

-- name: SetCariAccountActive :one
UPDATE cari_accounts
SET active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Ledger entries.

-- InsertFinanceEntry appends an original row. A retried sourced write (same
-- organization, source, role and revision) conflicts with
-- uq_finance_entries_source and returns no row (pgx.ErrNoRows); the caller
-- then reads the existing row with GetFinanceEntryBySource. posted_at places
-- the row in the ledger order (created_at); NULL means now. Only an opening
-- balance (TEC-177) passes it: the row sits at the opening date.
-- name: InsertFinanceEntry :one
INSERT INTO finance_entries (
    organization_id, brand_id, account_id, cari_id, direction, category,
    orig_currency, orig_amount, currency, amount, rate, rate_date,
    source_type, source_uuid, role, revision, description, actor_user_id, created_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(account_id), sqlc.narg(cari_id),
    sqlc.arg(direction), sqlc.arg(category),
    sqlc.arg(orig_currency), sqlc.arg(orig_amount), sqlc.arg(currency), sqlc.arg(amount),
    sqlc.arg(rate), sqlc.arg(rate_date),
    sqlc.narg(source_type), sqlc.narg(source_uuid), sqlc.arg(role), sqlc.arg(revision),
    sqlc.narg(description), sqlc.narg(actor_user_id),
    COALESCE(sqlc.narg(posted_at)::timestamptz, NOW())
)
ON CONFLICT (organization_id, source_type, source_uuid, role, revision)
    WHERE reversal_of_id IS NULL
    DO NOTHING
RETURNING *;

-- InsertFinanceReversal appends the mirror row of entry reversal_of_id
-- (negated amounts, same targets and source). A second reversal of the same
-- entry conflicts with uq_finance_entries_reversal_of and returns no row.
-- name: InsertFinanceReversal :one
INSERT INTO finance_entries (
    organization_id, brand_id, account_id, cari_id, direction, category,
    orig_currency, orig_amount, currency, amount, rate, rate_date,
    source_type, source_uuid, role, revision, reversal_of_id, description, actor_user_id
)
SELECT o.organization_id, o.brand_id, o.account_id, o.cari_id, o.direction, o.category,
       o.orig_currency, -o.orig_amount, o.currency, -o.amount, o.rate, o.rate_date,
       o.source_type, o.source_uuid, o.role, o.revision, o.id,
       sqlc.narg(description), sqlc.narg(actor_user_id)
FROM finance_entries o
WHERE o.id = sqlc.arg(reversal_of_id)
  AND o.organization_id = sqlc.arg(organization_id)
  AND o.reversal_of_id IS NULL
ON CONFLICT (reversal_of_id) DO NOTHING
RETURNING *;

-- name: GetFinanceEntry :one
SELECT * FROM finance_entries
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: GetFinanceEntryByUUID :one
SELECT * FROM finance_entries WHERE uuid = sqlc.arg(uuid);

-- name: GetFinanceEntryBySource :one
SELECT * FROM finance_entries
WHERE organization_id = sqlc.arg(organization_id)
  AND source_type = sqlc.arg(source_type)::text
  AND source_uuid = sqlc.arg(source_uuid)::uuid
  AND role = sqlc.arg(role)
  AND revision = sqlc.arg(revision)
  AND reversal_of_id IS NULL;

-- GetMaxFinanceEntryRevisionBySource is the highest revision written for a
-- source in one organization (0: none). TEC-177 opens a new revision of an
-- opening balance once the previous one is reversed.
-- name: GetMaxFinanceEntryRevisionBySource :one
SELECT COALESCE(MAX(revision), 0)::int AS revision
FROM finance_entries
WHERE organization_id = sqlc.arg(organization_id)
  AND source_type = sqlc.arg(source_type)::text
  AND source_uuid = sqlc.arg(source_uuid)::uuid;

-- GetFinanceEntryReversal returns the reversal row of an entry, if any.
-- name: GetFinanceEntryReversal :one
SELECT * FROM finance_entries WHERE reversal_of_id = sqlc.arg(entry_id);

-- ListFinanceEntriesBySource lists every row of one source in every
-- organization (seller and buyer side, originals and reversals), so
-- VoidBySource can reverse what is still open.
-- name: ListFinanceEntriesBySource :many
SELECT * FROM finance_entries
WHERE source_type = sqlc.arg(source_type)::text AND source_uuid = sqlc.arg(source_uuid)::uuid
ORDER BY organization_id, id;

-- ListOpenFinanceEntriesBySource lists the original rows of one source that
-- have not been reversed yet, locked for the reversing transaction.
-- name: ListOpenFinanceEntriesBySource :many
SELECT e.* FROM finance_entries e
WHERE e.source_type = sqlc.arg(source_type)::text AND e.source_uuid = sqlc.arg(source_uuid)::uuid
  AND e.reversal_of_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM finance_entries r WHERE r.reversal_of_id = e.id)
ORDER BY e.organization_id, e.id
FOR UPDATE OF e;

-- name: ListFinanceEntries :many
SELECT * FROM finance_entries
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction)::text)
  AND (sqlc.narg(account_id)::bigint IS NULL OR account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(cari_id)::bigint IS NULL OR cari_id = sqlc.narg(cari_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at < sqlc.narg(created_to)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListCariEntries :many
SELECT * FROM finance_entries
WHERE cari_id = sqlc.arg(cari_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- ---------------------------------------------------------------------------
-- Balances (views over the ledger).

-- name: GetCariBalance :one
SELECT * FROM cari_account_balances
WHERE cari_id = sqlc.arg(cari_id) AND organization_id = sqlc.arg(organization_id);

-- name: ListCariBalances :many
SELECT * FROM cari_account_balances
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY cari_id;

-- name: GetFinanceAccountBalance :one
SELECT * FROM finance_account_balances
WHERE account_id = sqlc.arg(account_id) AND organization_id = sqlc.arg(organization_id);

-- name: ListFinanceAccountBalances :many
SELECT * FROM finance_account_balances
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY account_id;
