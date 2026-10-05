-- TEC-312 (F3-03a): lead and quote schema (migration 000087). Reads are
-- bounded by resolved organization ids; the API layer owns scope resolution.

-- name: CreateLead :one
INSERT INTO leads (
    organization_id, brand_id, target_type, customer_user_id, vehicle_id,
    candidate_company_name, candidate_contact_name, candidate_phone_e164,
    candidate_email, country_id, province_id, district_id, source,
    temperature, status, lost_reason, follow_up_date, assignee_user_id,
    notes, won_ref_type, won_ref_id, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(target_type),
    sqlc.narg(customer_user_id), sqlc.narg(vehicle_id),
    sqlc.narg(candidate_company_name), sqlc.narg(candidate_contact_name),
    sqlc.narg(candidate_phone_e164), sqlc.narg(candidate_email),
    sqlc.narg(country_id), sqlc.narg(province_id), sqlc.narg(district_id),
    sqlc.arg(source), sqlc.arg(temperature), sqlc.arg(status),
    sqlc.narg(lost_reason), sqlc.narg(follow_up_date), sqlc.narg(assignee_user_id),
    sqlc.arg(notes), sqlc.narg(won_ref_type), sqlc.narg(won_ref_id),
    sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetLeadByUUID :one
SELECT * FROM leads
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL;

-- name: GetLeadByID :one
SELECT * FROM leads
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL;

-- name: GetLeadByIDForUpdate :one
-- TEC-316: lead conversion serializes on the lead row.
SELECT * FROM leads
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL
FOR UPDATE;

-- name: ListLeadsByOrganizations :many
SELECT * FROM leads
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
  AND (sqlc.narg(target_type)::varchar IS NULL OR target_type = sqlc.narg(target_type)::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountLeadsByOrganizations :one
SELECT COUNT(*) FROM leads
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
  AND (sqlc.narg(target_type)::varchar IS NULL OR target_type = sqlc.narg(target_type)::varchar);

-- name: ListLeadsInScope :many
SELECT * FROM leads
WHERE brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
  AND (sqlc.narg(target_type)::varchar IS NULL OR target_type = sqlc.narg(target_type)::varchar)
  AND (sqlc.narg(q)::text IS NULL OR (
       candidate_company_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR candidate_contact_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR candidate_phone_e164 ILIKE '%' || sqlc.narg(q)::text || '%'
       OR candidate_email ILIKE '%' || sqlc.narg(q)::text || '%'
       OR notes ILIKE '%' || sqlc.narg(q)::text || '%'
  ))
  AND (sqlc.arg(uuids)::uuid[] IS NULL OR uuid = ANY(sqlc.arg(uuids)::uuid[]))
  AND (NOT sqlc.arg(follow_up_only)::boolean OR (
       status IN ('new', 'contacted', 'quoted')
       AND follow_up_date IS NOT NULL
       AND (assignee_user_id IS NULL OR assignee_user_id = sqlc.arg(actor_user_id)::bigint)
       AND (sqlc.narg(follow_up_from)::timestamptz IS NULL OR follow_up_date >= sqlc.narg(follow_up_from)::timestamptz)
       AND (sqlc.narg(follow_up_to)::timestamptz IS NULL OR follow_up_date < sqlc.narg(follow_up_to)::timestamptz)
  ))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountLeadsInScope :one
SELECT COUNT(*) FROM leads
WHERE brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
  AND (sqlc.narg(target_type)::varchar IS NULL OR target_type = sqlc.narg(target_type)::varchar)
  AND (sqlc.narg(q)::text IS NULL OR (
       candidate_company_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR candidate_contact_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR candidate_phone_e164 ILIKE '%' || sqlc.narg(q)::text || '%'
       OR candidate_email ILIKE '%' || sqlc.narg(q)::text || '%'
       OR notes ILIKE '%' || sqlc.narg(q)::text || '%'
  ))
  AND (NOT sqlc.arg(follow_up_only)::boolean OR (
       status IN ('new', 'contacted', 'quoted')
       AND follow_up_date IS NOT NULL
       AND (assignee_user_id IS NULL OR assignee_user_id = sqlc.arg(actor_user_id)::bigint)
       AND (sqlc.narg(follow_up_from)::timestamptz IS NULL OR follow_up_date >= sqlc.narg(follow_up_from)::timestamptz)
       AND (sqlc.narg(follow_up_to)::timestamptz IS NULL OR follow_up_date < sqlc.narg(follow_up_to)::timestamptz)
  ));

-- name: GetLeadForIndex :one
SELECT * FROM leads
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: ListLeadsForIndex :many
SELECT * FROM leads
WHERE deleted_at IS NULL;

-- name: UpdateLead :one
UPDATE leads
SET target_type = sqlc.arg(target_type),
    customer_user_id = sqlc.narg(customer_user_id),
    vehicle_id = sqlc.narg(vehicle_id),
    candidate_company_name = sqlc.narg(candidate_company_name),
    candidate_contact_name = sqlc.narg(candidate_contact_name),
    candidate_phone_e164 = sqlc.narg(candidate_phone_e164),
    candidate_email = sqlc.narg(candidate_email),
    country_id = sqlc.narg(country_id),
    province_id = sqlc.narg(province_id),
    district_id = sqlc.narg(district_id),
    source = sqlc.arg(source),
    temperature = sqlc.arg(temperature),
    lost_reason = sqlc.narg(lost_reason),
    follow_up_date = sqlc.narg(follow_up_date),
    assignee_user_id = sqlc.narg(assignee_user_id),
    notes = sqlc.arg(notes)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: SetLeadStatus :one
UPDATE leads
SET status = sqlc.arg(status),
    lost_reason = sqlc.narg(lost_reason),
    won_ref_type = sqlc.narg(won_ref_type),
    won_ref_id = sqlc.narg(won_ref_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: AssignLead :one
UPDATE leads
SET assignee_user_id = sqlc.narg(assignee_user_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: SetLeadFollowUp :one
UPDATE leads
SET follow_up_date = sqlc.narg(follow_up_date)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteLead :execrows
UPDATE leads
SET deleted_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL;

-- name: AddLeadEvent :one
INSERT INTO lead_events (lead_id, organization_id, brand_id, event_type, payload, actor_user_id)
VALUES (
    sqlc.arg(lead_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(event_type),
    sqlc.arg(payload), sqlc.narg(actor_user_id)
)
RETURNING *;

-- name: AddQuoteViewedEventIfMissing :one
INSERT INTO lead_events (lead_id, organization_id, brand_id, event_type, payload)
SELECT
    sqlc.arg(lead_id), sqlc.arg(organization_id), sqlc.arg(brand_id), 'message',
    sqlc.arg(payload)
WHERE NOT EXISTS (
    SELECT 1
    FROM lead_events
    WHERE lead_id = sqlc.arg(lead_id)
      AND payload->>'kind' = 'quote_viewed'
      AND payload->>'quote_uuid' = sqlc.arg(quote_uuid)::text
)
RETURNING *;

-- name: ListLeadEvents :many
SELECT * FROM lead_events
WHERE lead_id = sqlc.arg(lead_id)
ORDER BY created_at, id;

-- name: NextQuoteNo :one
SELECT COALESCE(MAX(quote_no), 0)::bigint + 1 AS quote_no
FROM quotes
WHERE organization_id = sqlc.arg(organization_id);

-- name: CreateQuote :one
INSERT INTO quotes (
    organization_id, brand_id, lead_id, quote_no, currency, subtotal,
    discount_total, tax_total, grand_total, valid_until, status, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(lead_id), sqlc.arg(quote_no),
    sqlc.arg(currency), sqlc.arg(subtotal), sqlc.arg(discount_total),
    sqlc.arg(tax_total), sqlc.arg(grand_total), sqlc.narg(valid_until),
    sqlc.arg(status), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetQuoteByUUID :one
SELECT * FROM quotes
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL;

-- name: GetQuoteByPublicToken :one
SELECT * FROM quotes
WHERE public_token = sqlc.arg(public_token) AND deleted_at IS NULL;

-- name: GetQuotePublicViewByToken :one
SELECT
    q.id, q.uuid, q.organization_id, q.brand_id, q.lead_id, q.quote_no, q.currency,
    q.subtotal, q.discount_total, q.tax_total, q.grand_total, q.valid_until,
    q.status, q.public_token, q.created_at, q.updated_at,
    o.name AS organization_name
FROM quotes q
JOIN organizations o ON o.id = q.organization_id
WHERE q.public_token = sqlc.arg(public_token)
  AND q.brand_id = sqlc.arg(brand_id)
  AND q.status = 'sent'
  AND q.deleted_at IS NULL
  AND (q.valid_until IS NULL OR q.valid_until >= sqlc.arg(today)::date);

-- name: GetQuoteRecipient :one
SELECT
    l.candidate_phone_e164,
    u.id AS customer_user_id,
    u.phone_e164 AS customer_phone_e164,
    COALESCE(NULLIF(u.locale, ''), NULLIF(o.locale, ''), NULLIF(c.locale, ''), 'tr')::text AS language,
    COALESCE(NULLIF(l.candidate_contact_name, ''), NULLIF(l.candidate_company_name, ''), NULLIF(u.name || ' ' || u.surname, ' '), 'Müşteri')::text AS recipient_name,
    o.name AS organization_name
FROM quotes q
JOIN leads l ON l.id = q.lead_id
JOIN organizations o ON o.id = q.organization_id
LEFT JOIN organizations c ON c.brand_id = q.brand_id AND c.type = 'center'
LEFT JOIN users u ON u.id = l.customer_user_id
WHERE q.id = sqlc.arg(quote_id)
  AND q.organization_id = sqlc.arg(organization_id)
  AND q.deleted_at IS NULL;

-- name: ListQuotesByLead :many
SELECT * FROM quotes
WHERE lead_id = sqlc.arg(lead_id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: ListQuotesByOrganizations :many
SELECT * FROM quotes
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateQuoteTotals :one
UPDATE quotes
SET currency = sqlc.arg(currency),
    subtotal = sqlc.arg(subtotal),
    discount_total = sqlc.arg(discount_total),
    tax_total = sqlc.arg(tax_total),
    grand_total = sqlc.arg(grand_total),
    valid_until = sqlc.narg(valid_until)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'draft'
RETURNING *;

-- name: SetQuoteStatus :one
UPDATE quotes
SET status = sqlc.arg(status),
    sent_at = CASE WHEN sqlc.arg(status)::varchar = 'sent' THEN COALESCE(sent_at, NOW()) ELSE sent_at END,
    accepted_at = CASE WHEN sqlc.arg(status)::varchar = 'accepted' THEN COALESCE(accepted_at, NOW()) ELSE accepted_at END,
    rejected_at = CASE WHEN sqlc.arg(status)::varchar = 'rejected' THEN COALESCE(rejected_at, NOW()) ELSE rejected_at END,
    expired_at = CASE WHEN sqlc.arg(status)::varchar = 'expired' THEN COALESCE(expired_at, NOW()) ELSE expired_at END
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: EnsureQuoteSent :one
UPDATE quotes
SET status = 'sent',
    sent_at = COALESCE(sent_at, NOW())
WHERE id = sqlc.arg(id)
  AND organization_id = sqlc.arg(organization_id)
  AND status IN ('draft', 'sent')
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteQuote :execrows
UPDATE quotes
SET deleted_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'draft' AND deleted_at IS NULL;

-- name: CreateQuoteLine :one
INSERT INTO quote_lines (
    quote_id, organization_id, brand_id, line_type, product_id, service_catalog_item_id,
    description_snapshot, quantity, unit_price, discount_amount, line_total, sort_order
) VALUES (
    sqlc.arg(quote_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(line_type),
    sqlc.narg(product_id), sqlc.narg(service_catalog_item_id), sqlc.arg(description_snapshot),
    sqlc.arg(quantity), sqlc.arg(unit_price), sqlc.arg(discount_amount),
    sqlc.arg(line_total), sqlc.arg(sort_order)
)
RETURNING *;

-- name: ListQuoteLines :many
SELECT * FROM quote_lines
WHERE quote_id = sqlc.arg(quote_id)
ORDER BY sort_order, id;

-- name: DeleteQuoteLines :execrows
DELETE FROM quote_lines
WHERE quote_id = sqlc.arg(quote_id);

-- name: CreateQuoteDelivery :one
INSERT INTO quote_deliveries (quote_id, organization_id, brand_id, channel, status, provider_ref, error_message, sent_at)
VALUES (
    sqlc.arg(quote_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(channel),
    sqlc.arg(status), sqlc.narg(provider_ref), sqlc.narg(error_message), sqlc.narg(sent_at)
)
RETURNING *;

-- name: UpdateQuoteDeliveryStatus :one
UPDATE quote_deliveries
SET status = sqlc.arg(status),
    provider_ref = sqlc.narg(provider_ref),
    error_message = sqlc.narg(error_message),
    sent_at = sqlc.narg(sent_at)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: ListQuoteDeliveries :many
SELECT * FROM quote_deliveries
WHERE quote_id = sqlc.arg(quote_id)
ORDER BY created_at DESC, id DESC;

-- name: CreateQuoteReminder :one
INSERT INTO quote_reminders (quote_id, organization_id, brand_id, scheduled_at)
VALUES (sqlc.arg(quote_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(scheduled_at))
RETURNING *;

-- name: CreateQuoteReminderIfMissing :one
INSERT INTO quote_reminders (quote_id, organization_id, brand_id, scheduled_at)
SELECT sqlc.arg(quote_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(scheduled_at)
WHERE NOT EXISTS (
    SELECT 1 FROM quote_reminders WHERE quote_id = sqlc.arg(quote_id)
)
RETURNING *;

-- name: MarkQuoteReminderSent :one
UPDATE quote_reminders
SET sent_at = GREATEST(NOW(), scheduled_at)
WHERE id = sqlc.arg(id) AND sent_at IS NULL
RETURNING *;

-- name: GetQuoteReminderByID :one
SELECT * FROM quote_reminders
WHERE id = sqlc.arg(id);

-- name: ListDueQuoteReminders :many
SELECT * FROM quote_reminders
WHERE sent_at IS NULL AND scheduled_at <= sqlc.arg(now)::timestamptz
ORDER BY scheduled_at, id
LIMIT sqlc.arg(page_limit);

-- name: ExpireDueQuotes :many
UPDATE quotes
SET status = 'expired',
    expired_at = COALESCE(expired_at, NOW())
WHERE status IN ('draft', 'sent')
  AND valid_until IS NOT NULL
  AND valid_until < sqlc.arg(today)::date
  AND deleted_at IS NULL
RETURNING *;

-- name: LockQuoteNumbering :exec
-- Serializes quote number allocation per organization (transaction scoped).
SELECT pg_advisory_xact_lock(hashtextextended('quotes:' || sqlc.arg(organization_id)::bigint::text, 314));

-- name: LockQuoteByID :one
SELECT * FROM quotes
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
FOR UPDATE;
