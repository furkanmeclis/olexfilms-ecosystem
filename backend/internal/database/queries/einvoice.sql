-- TEC-501 (F5-08a): e-Invoice (UBL-TR) settings, counters, archive and
-- billable source records. Integrator submission is out of scope for F5.

-- Settings ------------------------------------------------------------------

-- name: UpsertEinvoiceSettings :one
INSERT INTO einvoice_settings (
    organization_id, brand_id, vkn, tax_office, legal_name, address, city,
    district, country, iban, email, phone, website, trade_registry_no,
    mersis_no, default_note, earchive_series, efatura_series,
    xslt_storage_key, xslt_sha1, pdf_enabled
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(vkn), sqlc.arg(tax_office),
    sqlc.arg(legal_name), sqlc.arg(address), sqlc.arg(city), sqlc.arg(district),
    sqlc.arg(country), sqlc.narg(iban), sqlc.narg(email), sqlc.narg(phone),
    sqlc.narg(website), sqlc.narg(trade_registry_no), sqlc.narg(mersis_no),
    sqlc.narg(default_note), sqlc.arg(earchive_series), sqlc.arg(efatura_series),
    sqlc.narg(xslt_storage_key), sqlc.narg(xslt_sha1), sqlc.arg(pdf_enabled)
)
ON CONFLICT (organization_id) DO UPDATE
SET vkn = EXCLUDED.vkn,
    tax_office = EXCLUDED.tax_office,
    legal_name = EXCLUDED.legal_name,
    address = EXCLUDED.address,
    city = EXCLUDED.city,
    district = EXCLUDED.district,
    country = EXCLUDED.country,
    iban = EXCLUDED.iban,
    email = EXCLUDED.email,
    phone = EXCLUDED.phone,
    website = EXCLUDED.website,
    trade_registry_no = EXCLUDED.trade_registry_no,
    mersis_no = EXCLUDED.mersis_no,
    default_note = EXCLUDED.default_note,
    earchive_series = EXCLUDED.earchive_series,
    efatura_series = EXCLUDED.efatura_series,
    xslt_storage_key = EXCLUDED.xslt_storage_key,
    xslt_sha1 = EXCLUDED.xslt_sha1,
    pdf_enabled = EXCLUDED.pdf_enabled
RETURNING *;

-- name: GetEinvoiceSettingsByOrg :one
SELECT * FROM einvoice_settings
WHERE organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id);

-- Counters ------------------------------------------------------------------

-- name: IncrementEinvoiceCounter :one
INSERT INTO einvoice_counters (organization_id, brand_id, series, year, last_no)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(series), sqlc.arg(year), 1)
ON CONFLICT (organization_id, series, year) DO UPDATE
SET last_no = einvoice_counters.last_no + 1
RETURNING organization_id, brand_id, series, year, last_no,
          (series || year::text || lpad(last_no::text, 9, '0'))::varchar(16) AS number;

-- name: GetEinvoiceCounter :one
SELECT * FROM einvoice_counters
WHERE organization_id = sqlc.arg(organization_id)
  AND series = sqlc.arg(series)
  AND year = sqlc.arg(year);

-- Archive -------------------------------------------------------------------

-- name: CreateEinvoice :one
INSERT INTO einvoices (
    uuid, organization_id, brand_id, number, profile, invoice_type,
    source_type, source_uuid, buyer_org_id, buyer, seller, lines, currency,
    rate_snapshot, line_extension, tax_exclusive, tax_total, payable,
    tax_breakdown, xml_storage_key, xml_sha256, pdf_storage_key,
    validation_status, validation_messages, status, error, issue_date, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(number),
    sqlc.arg(profile), sqlc.arg(invoice_type), sqlc.arg(source_type), sqlc.arg(source_uuid),
    sqlc.narg(buyer_org_id), sqlc.arg(buyer), sqlc.arg(seller), sqlc.arg(lines),
    sqlc.arg(currency), sqlc.arg(rate_snapshot), sqlc.arg(line_extension),
    sqlc.arg(tax_exclusive), sqlc.arg(tax_total), sqlc.arg(payable),
    sqlc.arg(tax_breakdown), sqlc.narg(xml_storage_key), sqlc.narg(xml_sha256),
    sqlc.narg(pdf_storage_key), sqlc.arg(validation_status),
    sqlc.arg(validation_messages), sqlc.arg(status), sqlc.narg(error),
    sqlc.arg(issue_date), sqlc.narg(created_by)
)
RETURNING *;

-- name: GetEinvoiceByUUID :one
SELECT * FROM einvoices
WHERE uuid = sqlc.arg(uuid)
  AND brand_id = sqlc.arg(brand_id);

-- name: GetActiveEinvoiceBySource :one
SELECT * FROM einvoices
WHERE organization_id = sqlc.arg(organization_id)
  AND source_type = sqlc.arg(source_type)
  AND source_uuid = sqlc.arg(source_uuid)
  AND status <> 'voided';

-- name: VoidEinvoice :one
UPDATE einvoices
SET status = 'voided',
    voided_at = NOW(),
    voided_by = sqlc.narg(voided_by),
    void_reason = sqlc.arg(void_reason)
WHERE id = sqlc.arg(id)
  AND brand_id = sqlc.arg(brand_id)
  AND status <> 'voided'
RETURNING *;

-- name: ListEinvoices :many
-- List contract: sort=issue_date|number|payable|status|created_at, default
-- -issue_date; id is the stable tiebreak. q matches number, buyer legal/name
-- fields and ETTN. status/profile/buyer_org_ids are multi-value filters.
SELECT e.*, bo.uuid AS buyer_org_uuid, bo.name AS buyer_org_name,
       COUNT(*) OVER()::bigint AS total_count
FROM einvoices e
LEFT JOIN organizations bo ON bo.id = e.buyer_org_id
WHERE e.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR e.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR e.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(profiles)::text[]), 0) = 0 OR e.profile = ANY (sqlc.narg(profiles)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(buyer_org_ids)::bigint[]), 0) = 0 OR e.buyer_org_id = ANY (sqlc.narg(buyer_org_ids)::bigint[]))
  AND (sqlc.narg(issue_date_from)::date IS NULL OR e.issue_date >= sqlc.narg(issue_date_from)::date)
  AND (sqlc.narg(issue_date_to)::date IS NULL OR e.issue_date <= sqlc.narg(issue_date_to)::date)
  AND (sqlc.narg(payable_min)::numeric IS NULL OR e.payable >= sqlc.narg(payable_min)::numeric)
  AND (sqlc.narg(payable_max)::numeric IS NULL OR e.payable <= sqlc.narg(payable_max)::numeric)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.number ILIKE '%' || sqlc.narg(q)::text || '%'
    OR e.uuid::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR bo.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR e.buyer ->> 'legal_name' ILIKE '%' || sqlc.narg(q)::text || '%'
    OR e.buyer ->> 'name' ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'issue_date' THEN e.issue_date END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'issue_date' THEN e.issue_date END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'number' THEN e.number WHEN 'status' THEN e.status END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'number' THEN e.number WHEN 'status' THEN e.status END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'payable' THEN e.payable END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'payable' THEN e.payable END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN e.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN e.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN e.id END DESC,
  e.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountEinvoices :one
SELECT COUNT(*)::bigint
FROM einvoices e
LEFT JOIN organizations bo ON bo.id = e.buyer_org_id
WHERE e.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR e.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR e.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(profiles)::text[]), 0) = 0 OR e.profile = ANY (sqlc.narg(profiles)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(buyer_org_ids)::bigint[]), 0) = 0 OR e.buyer_org_id = ANY (sqlc.narg(buyer_org_ids)::bigint[]))
  AND (sqlc.narg(issue_date_from)::date IS NULL OR e.issue_date >= sqlc.narg(issue_date_from)::date)
  AND (sqlc.narg(issue_date_to)::date IS NULL OR e.issue_date <= sqlc.narg(issue_date_to)::date)
  AND (sqlc.narg(payable_min)::numeric IS NULL OR e.payable >= sqlc.narg(payable_min)::numeric)
  AND (sqlc.narg(payable_max)::numeric IS NULL OR e.payable <= sqlc.narg(payable_max)::numeric)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.number ILIKE '%' || sqlc.narg(q)::text || '%'
    OR e.uuid::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR bo.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR e.buyer ->> 'legal_name' ILIKE '%' || sqlc.narg(q)::text || '%'
    OR e.buyer ->> 'name' ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- Billable sources -----------------------------------------------------------

-- name: ListEinvoiceBillableOrders :many
SELECT o.id, o.uuid, o.order_no AS source_no, o.buyer_org_id, buyer.name AS buyer_name,
       o.currency, o.rate_snapshot, o.subtotal AS line_extension,
       o.subtotal AS tax_exclusive, o.tax_total, o.total AS payable,
       o.received_at AS billable_at
FROM orders o
JOIN organizations seller ON seller.id = o.seller_org_id
JOIN organizations buyer ON buyer.id = o.buyer_org_id
LEFT JOIN einvoices e
  ON e.organization_id = o.seller_org_id
 AND e.source_type = 'order'
 AND e.source_uuid = o.uuid
 AND e.status <> 'voided'
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.seller_org_id = sqlc.arg(center_org_id)
  AND seller.type = 'center'
  AND buyer.type = 'distributor'
  AND o.status = 'received'
  AND e.id IS NULL
ORDER BY o.received_at ASC NULLS LAST, o.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListEinvoiceBillableSubscriptionPeriods :many
SELECT p.id, p.subscription_id, s.uuid AS subscription_uuid, i.name AS source_no,
       s.organization_id AS buyer_org_id, buyer.name AS buyer_name, s.currency,
       s.rate_snapshot, s.price AS line_extension, s.price AS tax_exclusive,
       0::numeric(18,2) AS tax_total, s.price AS payable, p.period_start, p.period_end,
       p.posted_at AS billable_at
FROM service_subscription_periods p
JOIN service_subscriptions s ON s.id = p.subscription_id
JOIN service_catalog_items i ON i.id = s.item_id
JOIN organizations seller ON seller.id = s.seller_org_id
JOIN organizations buyer ON buyer.id = s.organization_id
LEFT JOIN einvoices e
  ON e.organization_id = s.seller_org_id
 AND e.source_type = 'service_subscription'
 AND e.source_uuid = p.uuid
 AND e.status <> 'voided'
WHERE p.brand_id = sqlc.arg(brand_id)
  AND s.seller_org_id = sqlc.arg(center_org_id)
  AND seller.type = 'center'
  AND buyer.type = 'dealer'
  AND p.posted_at IS NOT NULL
  AND e.id IS NULL
ORDER BY p.period_start ASC, p.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
