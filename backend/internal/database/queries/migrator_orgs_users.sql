-- TEC-254: migrator step 1 (center, TR distributor, dealers, users, roles).
-- Written only by cmd/migrator inside a step transaction.

-- name: MigratorMatchProvince :one
-- Province by name, matched like the 000035 backfill: Turkish capitals
-- folded, case insensitive.
SELECT p.id FROM provinces p
WHERE p.country_id = sqlc.arg(country_id)::bigint
  AND lower(translate(p.name, 'İIıÇŞĞÜÖ', 'iiiçşğüö'))
      = lower(translate(btrim(sqlc.arg(name)::text), 'İIıÇŞĞÜÖ', 'iiiçşğüö'))
ORDER BY p.id
LIMIT 1;

-- name: MigratorMatchDistrict :one
SELECT d.id FROM districts d
WHERE d.province_id = sqlc.arg(province_id)::bigint
  AND lower(translate(d.name, 'İIıÇŞĞÜÖ', 'iiiçşğüö'))
      = lower(translate(btrim(sqlc.arg(name)::text), 'İIıÇŞĞÜÖ', 'iiiçşğüö'))
ORDER BY d.id
LIMIT 1;

-- name: MigratorCountryDistributor :one
-- The live distributor owning the country-level territory of a brand.
SELECT o.id FROM territories t
JOIN organizations o ON o.id = t.organization_id
WHERE t.brand_id = sqlc.arg(brand_id)::bigint
  AND t.country_id = sqlc.arg(country_id)::bigint
  AND t.province_id IS NULL
  AND o.type = 'distributor'
  AND o.deleted_at IS NULL
LIMIT 1;

-- name: MigratorDistributorBySlug :one
SELECT id FROM organizations
WHERE slug = sqlc.arg(slug) AND brand_id = sqlc.arg(brand_id)::bigint
  AND type = 'distributor' AND deleted_at IS NULL;

-- name: MigratorInsertOrganization :one
INSERT INTO organizations (
    uuid, slug, name, email, phone, phone_raw, address, city, district, website,
    status, plan_code, type, parent_id, brand_id, currency, locale, timezone,
    country_id, province_id, district_id, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(slug), sqlc.arg(name), sqlc.arg(email), sqlc.arg(phone),
    sqlc.narg(phone_raw), sqlc.arg(address), sqlc.arg(city), sqlc.arg(district), sqlc.arg(website),
    sqlc.arg(status), NULL, sqlc.arg(type), sqlc.narg(parent_id)::bigint, sqlc.arg(brand_id)::bigint,
    'TRY', 'tr', 'Europe/Istanbul',
    sqlc.narg(country_id)::bigint, sqlc.narg(province_id)::bigint, sqlc.narg(district_id)::bigint,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateOrganization :exec
-- Legacy-sourced fields only; slug and parent stay as the new app has them.
UPDATE organizations
SET name = sqlc.arg(name), email = sqlc.arg(email), phone = sqlc.arg(phone),
    phone_raw = sqlc.narg(phone_raw), address = sqlc.arg(address), city = sqlc.arg(city),
    district = sqlc.arg(district), website = sqlc.arg(website), status = sqlc.arg(status),
    country_id = sqlc.narg(country_id)::bigint, province_id = sqlc.narg(province_id)::bigint,
    district_id = sqlc.narg(district_id)::bigint
WHERE id = sqlc.arg(id)::bigint;

-- name: MigratorOrganizationIDByUUID :one
SELECT id FROM organizations WHERE uuid = $1;

-- name: MigratorUserByUUID :one
SELECT id, password_hash, email, phone_e164 FROM users WHERE uuid = $1;

-- name: MigratorFindUserByContact :one
-- An existing account with the e-mail (preferred) or the phone.
SELECT id, uuid FROM users
WHERE deleted_at IS NULL
  AND ((sqlc.narg(email)::text IS NOT NULL AND email = sqlc.narg(email)::text)
    OR (sqlc.narg(phone_e164)::text IS NOT NULL AND phone_e164 = sqlc.narg(phone_e164)::text))
ORDER BY (email IS NOT DISTINCT FROM sqlc.narg(email)::text) DESC, id
LIMIT 1;

-- name: MigratorContactTaken :one
-- Whether another live user already holds the e-mail or the phone.
SELECT
    EXISTS (SELECT 1 FROM users u WHERE u.deleted_at IS NULL AND u.id <> sqlc.arg(user_id)::bigint
            AND sqlc.narg(email)::text IS NOT NULL AND u.email = sqlc.narg(email)::text) AS email_taken,
    EXISTS (SELECT 1 FROM users u WHERE u.deleted_at IS NULL AND u.id <> sqlc.arg(user_id)::bigint
            AND sqlc.narg(phone_e164)::text IS NOT NULL AND u.phone_e164 = sqlc.narg(phone_e164)::text) AS phone_taken;

-- name: MigratorInsertUser :one
INSERT INTO users (
    uuid, email, password_hash, name, surname, status, email_verified_at, locale, phone_e164, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.narg(email)::text, sqlc.arg(password_hash), sqlc.arg(name), sqlc.arg(surname),
    sqlc.arg(status), sqlc.narg(email_verified_at)::timestamptz, sqlc.narg(locale)::text,
    sqlc.narg(phone_e164)::text, COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateUser :exec
-- The password is replaced only while the account still holds a migrated
-- hash (bcrypt or the reset marker); a password set in the new app wins.
UPDATE users
SET email = sqlc.narg(email)::text, name = sqlc.arg(name), surname = sqlc.arg(surname),
    status = sqlc.arg(status), locale = sqlc.narg(locale)::text, phone_e164 = sqlc.narg(phone_e164)::text,
    email_verified_at = COALESCE(email_verified_at, sqlc.narg(email_verified_at)::timestamptz),
    password_hash = CASE WHEN password_hash LIKE '$argon2id$%' THEN password_hash ELSE sqlc.arg(password_hash) END
WHERE id = sqlc.arg(id)::bigint;

-- name: MigratorEnsureMember :one
-- Adds the membership or returns the existing one; an owner grant upgrades
-- a staff membership, never the other way round.
INSERT INTO organization_members (organization_id, user_id, role)
VALUES (sqlc.arg(organization_id)::bigint, sqlc.arg(user_id)::bigint, sqlc.arg(role))
ON CONFLICT (organization_id, user_id) DO UPDATE
SET role = CASE WHEN EXCLUDED.role = 'owner' THEN 'owner' ELSE organization_members.role END
RETURNING id, (xmax = 0)::bool AS inserted;

-- name: MigratorAssignMemberRole :execrows
INSERT INTO organization_member_roles (member_id, role_id)
SELECT sqlc.arg(member_id)::bigint, r.id FROM roles r WHERE r.slug = sqlc.arg(slug)::text
ON CONFLICT DO NOTHING;

-- name: MigratorAssignUserRole :execrows
INSERT INTO user_roles (user_id, role_id)
SELECT sqlc.arg(user_id)::bigint, r.id FROM roles r WHERE r.slug = sqlc.arg(slug)::text
ON CONFLICT DO NOTHING;

-- name: MigratorRoleOrgTypes :many
SELECT slug, org_type FROM roles WHERE slug = ANY (sqlc.arg(slugs)::text[]);
