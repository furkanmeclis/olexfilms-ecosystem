-- TEC-174 (F1-07d): cari disputes (K24). A dispute is visible to the
-- disputing organization (organization_id) and to the parent it addresses
-- (counterparty_org_id); org_ids NULL means the whole brand (brand/all
-- scopes), otherwise any of org_ids must be one of the two sides.

-- InsertAccountingDispute opens a dispute. A second open dispute on the
-- same row conflicts with uq_accounting_disputes_open_entry and returns no
-- row.
-- name: InsertAccountingDispute :one
INSERT INTO accounting_disputes (
    organization_id, brand_id, counterparty_org_id, entry_id,
    source_type, source_uuid, reason, opened_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(counterparty_org_id), sqlc.arg(entry_id),
    sqlc.arg(source_type), sqlc.arg(source_uuid), sqlc.arg(reason), sqlc.narg(opened_by_user_id)
)
ON CONFLICT (entry_id) WHERE status = 'open' DO NOTHING
RETURNING *;

-- LockAccountingDispute locks a dispute addressed to the counterparty
-- organization for its resolution.
-- name: LockAccountingDispute :one
SELECT * FROM accounting_disputes
WHERE uuid = sqlc.arg(uuid) AND counterparty_org_id = sqlc.arg(counterparty_org_id)
FOR UPDATE;

-- name: ResolveAccountingDispute :one
UPDATE accounting_disputes
SET status = sqlc.arg(status),
    resolution_note = sqlc.narg(resolution_note),
    corrected_amount = sqlc.narg(corrected_amount),
    reversal_entry_id = sqlc.narg(reversal_entry_id),
    revision_entry_id = sqlc.narg(revision_entry_id),
    resolved_by_user_id = sqlc.narg(resolved_by_user_id),
    resolved_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'open'
RETURNING *;

-- name: ListAccountingDisputes :many
SELECT d.*,
       o.uuid AS organization_uuid, o.name AS organization_name,
       cp.uuid AS counterparty_org_uuid, cp.name AS counterparty_org_name,
       e.uuid AS entry_uuid, e.direction AS entry_direction, e.category AS entry_category,
       e.orig_currency AS entry_orig_currency, e.orig_amount AS entry_orig_amount,
       e.currency AS entry_currency, e.amount AS entry_amount, e.revision AS entry_revision,
       e.created_at AS entry_created_at,
       rv.uuid AS reversal_entry_uuid, rs.uuid AS revision_entry_uuid
FROM accounting_disputes d
JOIN organizations o ON o.id = d.organization_id
JOIN organizations cp ON cp.id = d.counterparty_org_id
JOIN finance_entries e ON e.id = d.entry_id
LEFT JOIN finance_entries rv ON rv.id = d.reversal_entry_id
LEFT JOIN finance_entries rs ON rs.id = d.revision_entry_id
WHERE d.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR d.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR d.counterparty_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status)::text)
  AND (sqlc.narg(organization_id)::bigint IS NULL OR d.organization_id = sqlc.narg(organization_id)::bigint)
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountAccountingDisputes :one
SELECT COUNT(*) FROM accounting_disputes d
WHERE d.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR d.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR d.counterparty_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status)::text)
  AND (sqlc.narg(organization_id)::bigint IS NULL OR d.organization_id = sqlc.narg(organization_id)::bigint);

-- name: GetAccountingDisputeView :one
SELECT d.*,
       o.uuid AS organization_uuid, o.name AS organization_name,
       cp.uuid AS counterparty_org_uuid, cp.name AS counterparty_org_name,
       e.uuid AS entry_uuid, e.direction AS entry_direction, e.category AS entry_category,
       e.orig_currency AS entry_orig_currency, e.orig_amount AS entry_orig_amount,
       e.currency AS entry_currency, e.amount AS entry_amount, e.revision AS entry_revision,
       e.created_at AS entry_created_at,
       rv.uuid AS reversal_entry_uuid, rs.uuid AS revision_entry_uuid
FROM accounting_disputes d
JOIN organizations o ON o.id = d.organization_id
JOIN organizations cp ON cp.id = d.counterparty_org_id
JOIN finance_entries e ON e.id = d.entry_id
LEFT JOIN finance_entries rv ON rv.id = d.reversal_entry_id
LEFT JOIN finance_entries rs ON rs.id = d.revision_entry_id
WHERE d.uuid = sqlc.arg(uuid)
  AND d.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR d.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR d.counterparty_org_id = ANY (sqlc.narg(org_ids)::bigint[]));

-- TEC-229: locks the order a dispute reversal would reverse (by its
-- accounting source uuid) against a concurrent return receipt
-- (LockOrdersOfTransferRequest).
-- name: LockOrderForDisputeReversal :one
SELECT id FROM orders
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- TEC-229: booked return lines of an order (a received return whose line
-- was priced and not excluded); a dispute on that order's sale cannot then
-- be resolved with a reversal.
-- name: CountBookedReturnItemsOfOrder :one
SELECT COUNT(*)
FROM stock_transfer_request_items i
JOIN stock_transfer_requests r ON r.id = i.request_id
JOIN order_items oi ON oi.id = i.order_item_id
WHERE oi.order_id = sqlc.arg(order_id)
  AND r.kind = 'return'
  AND r.status = 'received'
  AND i.line_total IS NOT NULL
  AND NOT i.accounting_excluded;
