-- TEC-161 (F1-08c): KVKK/GDPR anonymization and personal data export
-- (K19, TEC-100 decision 2). Nothing here deletes a row: users, vehicles,
-- services and warranties stay; only personal fields are overwritten.

-- name: LockUserByUUID :one
SELECT * FROM users WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL FOR UPDATE;

-- Irreversible: name "Anonim", no surname or phone, a unique placeholder
-- e-mail that satisfies chk_users_email_or_phone, a password hash nobody
-- knows and status anonymized (every login path requires status active).
-- The WHERE clause makes a second call a no-op.
-- name: AnonymizeUser :one
UPDATE users
SET name              = 'Anonim',
    surname           = '',
    email             = 'anonymized+' || uuid::text || '@anonymized.invalid',
    email_verified_at = NULL,
    phone_e164        = NULL,
    phone_verified_at = NULL,
    password_hash     = sqlc.arg(password_hash),
    timezone          = NULL,
    status            = 'anonymized'
WHERE id = sqlc.arg(id) AND status <> 'anonymized'
RETURNING *;

-- Clears every personal profile field (identity numbers: ciphertext and
-- mask together); anonymized_at keeps the first anonymization instant.
-- name: AnonymizeCustomerProfile :one
UPDATE customer_profiles
SET company_name       = NULL,
    tax_office         = NULL,
    national_id_enc    = NULL,
    national_id_last4  = NULL,
    tax_no_enc         = NULL,
    tax_no_last4       = NULL,
    address            = '{}'::jsonb,
    notification_prefs = '{"whatsapp": false, "email": false, "sms": false, "push": false}'::jsonb,
    anonymized_at      = COALESCE(anonymized_at, NOW())
WHERE user_id = sqlc.arg(user_id)
RETURNING *;

-- Services of a customer for the data export (brand_id NULL: every brand).
-- name: ListCustomerExportServices :many
SELECT s.uuid, s.service_no, s.status, s.plate, s.plate_country, s.vin, s.model_year, s.km,
       s.package, s.completed_at, s.cancelled_at, s.created_at,
       o.name AS organization_name, cb.name AS car_brand_name, cm.name AS car_model_name
FROM services s
JOIN organizations o ON o.id = s.organization_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE s.customer_user_id = sqlc.arg(customer_user_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR s.brand_id = sqlc.narg(brand_id))
ORDER BY s.created_at DESC, s.id DESC
LIMIT 5000;

-- Warranties held by a customer for the data export.
-- name: ListCustomerExportWarranties :many
SELECT w.uuid, w.public_code, w.item_kind, w.status, w.start_at, w.end_at, w.voided_at, w.created_at,
       p.name AS product_name, p.sku AS product_sku, s.service_no,
       v.plate, v.plate_country, v.vin, o.name AS organization_name
FROM warranties w
JOIN products p ON p.id = w.product_id
JOIN services s ON s.id = w.service_id
JOIN vehicles v ON v.id = w.vehicle_id
JOIN organizations o ON o.id = w.organization_id
WHERE w.holder_user_id = sqlc.arg(holder_user_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR w.brand_id = sqlc.narg(brand_id))
ORDER BY w.start_at DESC, w.id DESC
LIMIT 5000;

-- Is the customer linked to an organization of the brand?
-- name: CustomerLinkedToBrand :one
SELECT EXISTS (
    SELECT 1 FROM customer_organizations
    WHERE user_id = sqlc.arg(user_id) AND brand_id = sqlc.arg(brand_id)
)::boolean AS linked;
