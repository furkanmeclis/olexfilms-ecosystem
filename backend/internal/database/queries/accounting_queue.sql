-- TEC-1070: accounting queue (legacy olexfilms-warehouse "Muhasebe
-- kuyruğu") of one book: orders it sold that wait for an invoice mark
-- (TEC-1030) and returns it accepted that wait to be processed.

-- name: AccountingQueueCounts :one
-- to_invoice: received seller orders not fully invoiced (the invoice mark
-- needs a received order); in_transit: shipped or delivered seller orders
-- not invoiced yet (they enter to_invoice when the buyer receives them);
-- returns: received returns to the book not processed yet.
SELECT
    (SELECT COUNT(*) FROM orders o
      WHERE o.seller_org_id = sqlc.arg(organization_id)::bigint
        AND o.status = 'received' AND o.invoice_status <> 'invoiced')::bigint AS to_invoice,
    (SELECT COUNT(*) FROM orders o
      WHERE o.seller_org_id = sqlc.arg(organization_id)::bigint
        AND o.status IN ('shipped', 'delivered') AND o.invoice_status <> 'invoiced')::bigint AS in_transit,
    (SELECT COUNT(*) FROM stock_transfer_requests t
      WHERE t.to_org_id = sqlc.arg(organization_id)::bigint
        AND t.kind = 'return' AND t.status = 'received'
        AND t.accounting_processed_at IS NULL)::bigint AS returns_pending;

-- name: ListAccountingQueueReturns :many
-- processed: NULL every received return, true processed ones, false waiting.
-- sort_key received_at (default), transfer_no, processed_at; id tiebreak.
SELECT t.uuid, t.transfer_no, t.currency, COALESCE(t.total::text, '')::text AS total, t.received_at, t.created_at,
       t.accounting_processed_at, t.accounting_note,
       btrim(COALESCE(pu.name, '') || ' ' || COALESCE(pu.surname, ''))::text AS processed_by_name,
       f.uuid AS sender_uuid, f.name AS sender_name, f.type AS sender_type,
       (SELECT COUNT(*) FROM stock_transfer_request_items i WHERE i.request_id = t.id)::bigint AS item_count,
       ro.uuid AS order_uuid, ro.order_no AS order_no
FROM stock_transfer_requests t
JOIN organizations f ON f.id = t.from_org_id
LEFT JOIN users pu ON pu.id = t.accounting_processed_by_user_id
-- TEC-1085: the order the return refers to.
LEFT JOIN orders ro ON ro.id = t.order_id
WHERE t.to_org_id = sqlc.arg(organization_id)::bigint
  AND t.kind = 'return' AND t.status = 'received'
  AND (sqlc.narg(processed)::bool IS NULL
       OR (t.accounting_processed_at IS NOT NULL) = sqlc.narg(processed)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR t.transfer_no ILIKE sqlc.narg(q)::text || '%'
       OR ro.order_no ILIKE sqlc.narg(q)::text || '%'
       OR f.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'transfer_no' THEN t.transfer_no END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'transfer_no' THEN t.transfer_no END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'received_at' THEN t.received_at WHEN 'processed_at' THEN t.accounting_processed_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'received_at' THEN t.received_at WHEN 'processed_at' THEN t.accounting_processed_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAccountingQueueReturns :one
SELECT COUNT(*)::bigint
FROM stock_transfer_requests t
JOIN organizations f ON f.id = t.from_org_id
LEFT JOIN orders ro ON ro.id = t.order_id
WHERE t.to_org_id = sqlc.arg(organization_id)::bigint
  AND t.kind = 'return' AND t.status = 'received'
  AND (sqlc.narg(processed)::bool IS NULL
       OR (t.accounting_processed_at IS NOT NULL) = sqlc.narg(processed)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR t.transfer_no ILIKE sqlc.narg(q)::text || '%'
       OR ro.order_no ILIKE sqlc.narg(q)::text || '%'
       OR f.name ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: SetReturnAccountingProcessed :one
-- Marks (processed = true) or clears the accounting mark of a received
-- return to the book. No row: not found, not a received return of the book.
UPDATE stock_transfer_requests t
SET accounting_processed_at = CASE WHEN sqlc.arg(processed)::bool THEN NOW() END,
    accounting_processed_by_user_id = CASE WHEN sqlc.arg(processed)::bool THEN sqlc.narg(user_id)::bigint END,
    accounting_note = CASE WHEN sqlc.arg(processed)::bool THEN sqlc.narg(note)::text END,
    updated_at = NOW()
WHERE t.uuid = sqlc.arg(uuid)
  AND t.to_org_id = sqlc.arg(organization_id)::bigint
  AND t.kind = 'return' AND t.status = 'received'
RETURNING t.uuid;

-- name: GetOrganizationBilling :one
SELECT o.id, o.uuid, o.name, o.billing_title, o.tax_office, o.tax_no, o.billing_address,
       o.billing_email, o.iban, o.billing_updated_at, btrim(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, ''))::text AS billing_updated_by_name
FROM organizations o
LEFT JOIN users u ON u.id = o.billing_updated_by_user_id
WHERE o.id = sqlc.arg(id);

-- name: UpdateOrganizationBilling :exec
UPDATE organizations
SET billing_title = sqlc.narg(billing_title),
    tax_office = sqlc.narg(tax_office),
    tax_no = sqlc.narg(tax_no),
    billing_address = sqlc.narg(billing_address),
    billing_email = sqlc.narg(billing_email),
    iban = sqlc.narg(iban),
    billing_updated_at = NOW(),
    billing_updated_by_user_id = sqlc.narg(user_id),
    updated_at = NOW()
WHERE id = sqlc.arg(id);
