-- TEC-1062: the center's waiting pool of recipients (migration 000169).

-- name: CreatePendingRecipient :one
INSERT INTO pending_recipients (
    organization_id, brand_id, recipient_org_id, kind, note, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(recipient_org_id), sqlc.arg(kind),
    sqlc.narg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetPendingRecipientByOrg :one
-- The pool row of a recipient organization, any state.
SELECT * FROM pending_recipients
WHERE recipient_org_id = sqlc.arg(recipient_org_id);

-- name: GetPendingRecipientForUpdate :one
-- The pool row of a recipient of the center, locked for a state change.
SELECT pr.* FROM pending_recipients pr
JOIN organizations o ON o.id = pr.recipient_org_id AND o.deleted_at IS NULL
WHERE pr.organization_id = sqlc.arg(organization_id) AND o.uuid = sqlc.arg(recipient_uuid)
FOR UPDATE OF pr;

-- name: ResolvePendingRecipient :one
UPDATE pending_recipients
SET state = sqlc.arg(state),
    resolved_by_user_id = sqlc.narg(resolved_by_user_id),
    resolved_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetRecipientOrganizationStatus :one
UPDATE organizations
SET status = sqlc.arg(status), updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetRecipientInvoiceTax :exec
-- The tax identity typed with the recipient (10 digits: VKN, 11: TCKN).
UPDATE organizations
SET invoice_vkn = sqlc.narg(invoice_vkn),
    invoice_tckn = sqlc.narg(invoice_tckn),
    invoice_tax_office = sqlc.narg(invoice_tax_office),
    invoice_legal_name = sqlc.narg(invoice_legal_name),
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: CountOrdersOfBuyer :one
SELECT COUNT(*)::bigint FROM orders WHERE buyer_org_id = sqlc.arg(buyer_org_id);

-- name: ListPendingRecipients :many
-- Sort: docs/list-contract.md, keys from usecase.PendingRecipientSort.
SELECT pr.id, pr.kind, pr.state, pr.note, pr.created_at, pr.resolved_at,
       o.uuid AS recipient_uuid, o.slug AS recipient_slug, o.name AS recipient_name,
       o.phone AS recipient_phone, o.city AS recipient_city, o.district AS recipient_district,
       o.address AS recipient_address, o.status AS recipient_status,
       o.invoice_vkn, o.invoice_tckn, o.invoice_tax_office,
       (SELECT COUNT(*) FROM orders ord WHERE ord.buyer_org_id = o.id)::bigint AS orders_count
FROM pending_recipients pr
JOIN organizations o ON o.id = pr.recipient_org_id AND o.deleted_at IS NULL
WHERE pr.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(states)::text[]), 0) = 0 OR pr.state = ANY (sqlc.narg(states)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0 OR pr.kind = ANY (sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.phone ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.city ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR pr.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR pr.created_at < sqlc.narg(created_before))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN o.name WHEN 'kind' THEN pr.kind WHEN 'state' THEN pr.state
      WHEN 'city' THEN o.city END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN o.name WHEN 'kind' THEN pr.kind WHEN 'state' THEN pr.state
      WHEN 'city' THEN o.city END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN pr.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN pr.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN pr.id END DESC,
  pr.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPendingRecipients :one
SELECT COUNT(*)::bigint
FROM pending_recipients pr
JOIN organizations o ON o.id = pr.recipient_org_id AND o.deleted_at IS NULL
WHERE pr.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(states)::text[]), 0) = 0 OR pr.state = ANY (sqlc.narg(states)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0 OR pr.kind = ANY (sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.phone ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.city ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR pr.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR pr.created_at < sqlc.narg(created_before));
