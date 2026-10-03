-- TEC-255: migrator step 2 (hub customers -> users, customer_profiles,
-- customer_organizations). Written only by cmd/migrator inside a step
-- transaction.

-- name: MigratorInsertCustomerUser :one
-- A legacy customer becomes a users row (K11). legacy_unverified marks a
-- customer without a resolved phone (K26, K29, migration 000078).
INSERT INTO users (
    uuid, email, password_hash, name, surname, status, phone_e164,
    legacy_unverified, legacy_phone_raw, created_at, deleted_at
) VALUES (
    sqlc.arg(uuid), sqlc.narg(email)::text, sqlc.arg(password_hash), sqlc.arg(name), sqlc.arg(surname),
    sqlc.arg(status), sqlc.narg(phone_e164)::text,
    sqlc.arg(legacy_unverified)::bool, sqlc.narg(legacy_phone_raw)::text,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW()), sqlc.narg(deleted_at)::timestamptz
)
RETURNING id;

-- name: MigratorFillCustomerUser :execrows
-- A merged or changed legacy customer only fills what the account lacks;
-- values set in the new app (or by an earlier source) are kept.
UPDATE users
SET email            = COALESCE(email, sqlc.narg(email)::text),
    phone_e164       = COALESCE(phone_e164, sqlc.narg(phone_e164)::text),
    legacy_phone_raw = CASE WHEN legacy_unverified
                            THEN COALESCE(legacy_phone_raw, sqlc.narg(legacy_phone_raw)::text)
                            ELSE legacy_phone_raw END
WHERE id = sqlc.arg(id)::bigint
  AND ((email IS NULL AND sqlc.narg(email)::text IS NOT NULL)
    OR (phone_e164 IS NULL AND sqlc.narg(phone_e164)::text IS NOT NULL)
    OR (legacy_unverified AND legacy_phone_raw IS NULL AND sqlc.narg(legacy_phone_raw)::text IS NOT NULL));

-- name: MigratorEnsureCustomerProfile :execrows
INSERT INTO customer_profiles (user_id, type, company_name, tax_office, address, notification_prefs, created_at)
VALUES (
    sqlc.arg(user_id)::bigint, sqlc.arg(type), sqlc.narg(company_name)::text, sqlc.narg(tax_office)::text,
    sqlc.arg(address)::jsonb, sqlc.arg(notification_prefs)::jsonb,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
ON CONFLICT (user_id) DO NOTHING;

-- name: MigratorFillCustomerProfile :execrows
-- Fill-only, like MigratorFillCustomerUser.
UPDATE customer_profiles
SET company_name = COALESCE(company_name, sqlc.narg(company_name)::text),
    tax_office   = COALESCE(tax_office, sqlc.narg(tax_office)::text),
    address      = CASE WHEN address = '{}'::jsonb THEN sqlc.arg(address)::jsonb ELSE address END
WHERE user_id = sqlc.arg(user_id)::bigint
  AND ((company_name IS NULL AND sqlc.narg(company_name)::text IS NOT NULL)
    OR (tax_office IS NULL AND sqlc.narg(tax_office)::text IS NOT NULL)
    OR (address = '{}'::jsonb AND sqlc.arg(address)::jsonb <> '{}'::jsonb));

-- name: MigratorLinkCustomerOrganization :execrows
-- One row per serving organization (K11: the customer is global, a dealer
-- sees it through this link).
INSERT INTO customer_organizations (user_id, organization_id, brand_id, created_at)
VALUES (
    sqlc.arg(user_id)::bigint, sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
ON CONFLICT (user_id, organization_id) DO NOTHING;
