-- TEC-159: customer profiles, customer x organization links and vehicles
-- (migration 000048). Scope arguments follow scopefilter: org_ids NULL means
-- no organization restriction (all/brand), an empty array matches nothing;
-- brand_id NULL means every brand.

-- name: CreateCustomerProfile :one
INSERT INTO customer_profiles (
    user_id, type, company_name, tax_office,
    national_id_enc, national_id_last4, tax_no_enc, tax_no_last4,
    address, notification_prefs
) VALUES (
    sqlc.arg(user_id), sqlc.arg(type), sqlc.narg(company_name), sqlc.narg(tax_office),
    sqlc.narg(national_id_enc), sqlc.narg(national_id_last4), sqlc.narg(tax_no_enc), sqlc.narg(tax_no_last4),
    sqlc.arg(address), sqlc.arg(notification_prefs)
)
RETURNING *;

-- name: EnsureCustomerProfile :exec
INSERT INTO customer_profiles (user_id) VALUES (sqlc.arg(user_id))
ON CONFLICT (user_id) DO NOTHING;

-- name: GetCustomerProfile :one
SELECT * FROM customer_profiles WHERE user_id = sqlc.arg(user_id);

-- name: GetCustomerProfileForUpdate :one
SELECT * FROM customer_profiles WHERE user_id = sqlc.arg(user_id) FOR UPDATE;

-- name: UpdateCustomerProfile :one
UPDATE customer_profiles
SET type               = sqlc.arg(type),
    company_name       = sqlc.narg(company_name),
    tax_office         = sqlc.narg(tax_office),
    address            = sqlc.arg(address),
    notification_prefs = sqlc.arg(notification_prefs)
WHERE user_id = sqlc.arg(user_id)
RETURNING *;

-- Ciphertext and mask are written together; NULL/NULL clears the value.
-- name: SetCustomerNationalID :one
UPDATE customer_profiles
SET national_id_enc = sqlc.narg(national_id_enc), national_id_last4 = sqlc.narg(national_id_last4)
WHERE user_id = sqlc.arg(user_id)
RETURNING *;

-- name: SetCustomerTaxNo :one
UPDATE customer_profiles
SET tax_no_enc = sqlc.narg(tax_no_enc), tax_no_last4 = sqlc.narg(tax_no_last4)
WHERE user_id = sqlc.arg(user_id)
RETURNING *;

-- Idempotent link: a second call keeps the row and fills first_service_at
-- only when it was empty.
-- name: LinkCustomerOrganization :one
INSERT INTO customer_organizations (user_id, organization_id, brand_id, first_service_at)
VALUES (sqlc.arg(user_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(first_service_at))
ON CONFLICT (user_id, organization_id) DO UPDATE
SET first_service_at = COALESCE(customer_organizations.first_service_at, EXCLUDED.first_service_at)
RETURNING *;

-- name: InsertCustomerOrganization :one
INSERT INTO customer_organizations (user_id, organization_id, brand_id, first_service_at)
VALUES (sqlc.arg(user_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(first_service_at))
RETURNING *;

-- name: MarkCustomerFirstService :execrows
UPDATE customer_organizations
SET first_service_at = sqlc.arg(first_service_at)
WHERE user_id = sqlc.arg(user_id)
  AND organization_id = sqlc.arg(organization_id)
  AND first_service_at IS NULL;

-- name: GetCustomerOrganization :one
SELECT * FROM customer_organizations
WHERE user_id = sqlc.arg(user_id) AND organization_id = sqlc.arg(organization_id);

-- Organizations serving a customer (portal, customer detail).
-- name: ListCustomerOrganizationsByUser :many
SELECT co.id, co.user_id, co.organization_id, co.brand_id, co.first_service_at, co.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       o.phone AS organization_phone, b.slug AS brand_slug
FROM customer_organizations co
JOIN organizations o ON o.id = co.organization_id
JOIN brands b ON b.id = co.brand_id
WHERE co.user_id = sqlc.arg(user_id)
  AND o.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(brand_id)::bigint IS NULL OR co.brand_id = sqlc.narg(brand_id))
ORDER BY co.created_at ASC, co.id ASC;

-- Scope check: is the customer linked to an organization the caller reaches?
-- name: CustomerInScope :one
SELECT EXISTS (
    SELECT 1 FROM customer_organizations co
    WHERE co.user_id = sqlc.arg(user_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
      AND (sqlc.narg(brand_id)::bigint IS NULL OR co.brand_id = sqlc.narg(brand_id))
)::boolean AS in_scope;

-- Customers of the organizations in scope; one row per user.
-- TEC-371: statuses / customer types are CSV filters, linked_at is a date
-- range on the first link, organization_uuids narrows the links to those
-- organizations (inside the scope above). Sort: docs/list-contract.md, keys
-- from customers usecase customersSortSpec.
-- name: ListOrganizationCustomers :many
SELECT u.id, u.uuid, u.name, u.surname, u.email, u.phone_e164, u.status, u.locale, u.created_at,
       cp.type AS customer_type, cp.company_name,
       MIN(co.created_at)::timestamptz AS linked_at,
       MIN(co.first_service_at)::timestamptz AS first_service_at
FROM users u
JOIN customer_organizations co ON co.user_id = u.id
LEFT JOIN customer_profiles cp ON cp.user_id = u.id
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(brand_id)::bigint IS NULL OR co.brand_id = sqlc.narg(brand_id))
  AND (
    sqlc.narg(organization_uuids)::uuid[] IS NULL
    OR co.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR u.status = ANY (sqlc.narg(statuses)::text[]))
  AND (
    COALESCE(cardinality(sqlc.narg(customer_types)::text[]), 0) = 0
    OR COALESCE(cp.type, 'individual') = ANY (sqlc.narg(customer_types)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.phone_e164 LIKE '%' || sqlc.narg(q) || '%'
    OR cp.company_name ILIKE '%' || sqlc.narg(q) || '%'
  )
  -- TEC-164: Meilisearch hits; the scope filter above still applies.
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR u.uuid = ANY (sqlc.narg(uuids)::uuid[]))
GROUP BY u.id, cp.user_id
HAVING (sqlc.narg(linked_from)::timestamptz IS NULL OR MIN(co.created_at) >= sqlc.narg(linked_from)::timestamptz)
   AND (sqlc.narg(linked_before)::timestamptz IS NULL OR MIN(co.created_at) < sqlc.narg(linked_before)::timestamptz)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(u.name || ' ' || u.surname) WHEN 'status' THEN u.status END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(u.name || ' ' || u.surname) WHEN 'status' THEN u.status END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'email' THEN lower(u.email) END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'email' THEN lower(u.email) END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'linked_at' THEN MIN(co.created_at) END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'linked_at' THEN MIN(co.created_at) END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'first_service_at' THEN MIN(co.first_service_at) END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'first_service_at' THEN MIN(co.first_service_at) END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN u.id END DESC,
  u.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationCustomers :one
SELECT COUNT(*)::bigint FROM (
  SELECT u.id
  FROM users u
  JOIN customer_organizations co ON co.user_id = u.id
  LEFT JOIN customer_profiles cp ON cp.user_id = u.id
  WHERE u.deleted_at IS NULL
    AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
    AND (sqlc.narg(brand_id)::bigint IS NULL OR co.brand_id = sqlc.narg(brand_id))
    AND (
      sqlc.narg(organization_uuids)::uuid[] IS NULL
      OR co.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
    )
    AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR u.status = ANY (sqlc.narg(statuses)::text[]))
    AND (
      COALESCE(cardinality(sqlc.narg(customer_types)::text[]), 0) = 0
      OR COALESCE(cp.type, 'individual') = ANY (sqlc.narg(customer_types)::text[])
    )
    AND (
      sqlc.narg(q)::text IS NULL
      OR u.name ILIKE '%' || sqlc.narg(q) || '%'
      OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
      OR u.email ILIKE '%' || sqlc.narg(q) || '%'
      OR u.phone_e164 LIKE '%' || sqlc.narg(q) || '%'
      OR cp.company_name ILIKE '%' || sqlc.narg(q) || '%'
    )
  GROUP BY u.id
  HAVING (sqlc.narg(linked_from)::timestamptz IS NULL OR MIN(co.created_at) >= sqlc.narg(linked_from)::timestamptz)
     AND (sqlc.narg(linked_before)::timestamptz IS NULL OR MIN(co.created_at) < sqlc.narg(linked_before)::timestamptz)
) matched;

-- name: CreateVehicle :one
INSERT INTO vehicles (
    user_id, organization_id, brand_id, car_brand_id, car_model_id, model_year,
    plate, plate_normalized, plate_country, vin
) VALUES (
    sqlc.arg(user_id), sqlc.narg(organization_id), sqlc.arg(brand_id), sqlc.narg(car_brand_id),
    sqlc.narg(car_model_id), sqlc.narg(model_year), sqlc.narg(plate), sqlc.narg(plate_normalized),
    sqlc.narg(plate_country), sqlc.narg(vin)
)
RETURNING *;

-- name: GetVehicleByUUID :one
SELECT * FROM vehicles WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: GetVehicleByUUIDForUpdate :one
SELECT * FROM vehicles WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL FOR UPDATE;

-- Full replace of the editable fields (the caller reads the row first).
-- name: UpdateVehicle :one
UPDATE vehicles
SET car_brand_id     = sqlc.narg(car_brand_id),
    car_model_id     = sqlc.narg(car_model_id),
    model_year       = sqlc.narg(model_year),
    plate            = sqlc.narg(plate),
    plate_normalized = sqlc.narg(plate_normalized),
    plate_country    = sqlc.narg(plate_country),
    vin              = sqlc.narg(vin)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteVehicle :execrows
UPDATE vehicles SET deleted_at = NOW() WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListVehiclesByUser :many
SELECT v.*, cb.name AS car_brand_name, cm.name AS car_model_name
FROM vehicles v
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.user_id = sqlc.arg(user_id)
  AND v.deleted_at IS NULL
  AND (sqlc.narg(brand_id)::bigint IS NULL OR v.brand_id = sqlc.narg(brand_id))
ORDER BY v.created_at DESC, v.id DESC;

-- Vehicles of customers linked to the organizations in scope.
-- name: ListVehiclesInScope :many
SELECT v.*, cb.name AS car_brand_name, cm.name AS car_model_name
FROM vehicles v
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.deleted_at IS NULL
  AND (sqlc.narg(user_id)::bigint IS NULL OR v.user_id = sqlc.narg(user_id))
  AND (sqlc.narg(brand_id)::bigint IS NULL OR v.brand_id = sqlc.narg(brand_id))
  AND EXISTS (
    SELECT 1 FROM customer_organizations co
    WHERE co.user_id = v.user_id
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
      AND (sqlc.narg(brand_id)::bigint IS NULL OR co.brand_id = sqlc.narg(brand_id))
  )
  AND (
    sqlc.narg(plate_normalized)::text IS NULL
    OR v.plate_normalized LIKE sqlc.narg(plate_normalized) || '%'
  )
ORDER BY v.created_at DESC, v.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- Plate lookup (not unique: plates change hands).
-- name: FindVehiclesByPlate :many
SELECT * FROM vehicles
WHERE plate_country = sqlc.arg(plate_country)
  AND plate_normalized = sqlc.arg(plate_normalized)
  AND deleted_at IS NULL
  AND (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id))
ORDER BY created_at DESC, id DESC;

-- Duplicate-VIN warning (VIN is not unique; ownership transfer is F1-06).
-- name: FindVehiclesByVIN :many
SELECT * FROM vehicles
WHERE vin = sqlc.arg(vin)
  AND deleted_at IS NULL
  AND (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id))
ORDER BY created_at DESC, id DESC;

-- K29 organization phone normalization (cmd/normalize-org-phones): rows
-- whose original phone was moved aside by migration 000048.
-- name: ListOrganizationsWithRawPhone :many
SELECT o.id, o.uuid, o.name, o.phone, o.phone_raw, c.iso2 AS country_iso2
FROM organizations o
LEFT JOIN countries c ON c.id = o.country_id
WHERE o.phone_raw IS NOT NULL
ORDER BY o.id ASC;

-- Success: phone gets the E.164 form and phone_raw is cleared. A phone
-- written by the API in the meantime is kept (only phone_raw is cleared).
-- name: ResolveOrganizationRawPhone :execrows
UPDATE organizations
SET phone = CASE WHEN phone = '' THEN sqlc.arg(phone_e164)::text ELSE phone END,
    phone_raw = NULL
WHERE id = sqlc.arg(id) AND phone_raw IS NOT NULL;

-- Country of an organization for the default phone region (K29).
-- name: GetOrganizationCountryISO2 :one
SELECT COALESCE(c.iso2, '')::text AS iso2
FROM organizations o
LEFT JOIN countries c ON c.id = o.country_id
WHERE o.id = sqlc.arg(id);

-- TEC-160 (F1-08b): customer and vehicle API (/v1/customers, /v1/vehicles).

-- Fill-only identity: a customer created by another organization keeps its
-- name, e-mail and locale; only empty values are filled.
-- name: FillCustomerIdentity :one
UPDATE users
SET name    = CASE WHEN btrim(name) = '' AND sqlc.narg(name)::text IS NOT NULL THEN sqlc.narg(name)::text ELSE name END,
    surname = CASE WHEN btrim(surname) = '' AND sqlc.narg(surname)::text IS NOT NULL THEN sqlc.narg(surname)::text ELSE surname END,
    email   = COALESCE(email, sqlc.narg(email)::text),
    locale  = COALESCE(locale, sqlc.narg(locale)::text)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- Full identity edit (only when the caller's scope covers every link of the
-- customer and the user has no panel membership).
-- name: SetCustomerIdentity :one
UPDATE users
SET name    = sqlc.arg(name),
    surname = sqlc.arg(surname),
    email   = sqlc.narg(email)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: CountCustomerOrganizationLinks :one
SELECT COUNT(*)::bigint AS total,
       (COUNT(*) FILTER (
         WHERE (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
           AND (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id))
       ))::bigint AS in_scope
FROM customer_organizations
WHERE user_id = sqlc.arg(user_id);

-- name: CountOrganizationMembershipsByUser :one
SELECT COUNT(*)::bigint FROM organization_members WHERE user_id = sqlc.arg(user_id);

-- Vehicle with its customer and car brand/model, for API responses.
-- name: GetVehicleViewByUUID :one
SELECT sqlc.embed(v), u.uuid AS customer_uuid,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cm.uuid AS car_model_uuid, cm.name AS car_model_name,
       o.uuid AS organization_uuid
FROM vehicles v
JOIN users u ON u.id = v.user_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
LEFT JOIN organizations o ON o.id = v.organization_id
WHERE v.uuid = sqlc.arg(uuid) AND v.deleted_at IS NULL;

-- Vehicles of customers linked to the organizations in scope; the brand is
-- always the domain brand (K20). TEC-371: q also matches the car brand /
-- model name (q_name), car_brand_uuids / car_model_uuids / organization_uuids
-- filters (the organization filter narrows the owner's links inside the
-- scope). Sort: docs/list-contract.md, keys from vehiclesSortSpec.
-- name: ListScopedVehicles :many
SELECT sqlc.embed(v), u.uuid AS customer_uuid,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cm.uuid AS car_model_uuid, cm.name AS car_model_name,
       o.uuid AS organization_uuid
FROM vehicles v
JOIN users u ON u.id = v.user_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
LEFT JOIN organizations o ON o.id = v.organization_id
WHERE v.deleted_at IS NULL
  AND v.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(user_id)::bigint IS NULL OR v.user_id = sqlc.narg(user_id))
  AND EXISTS (
    SELECT 1 FROM customer_organizations co
    WHERE co.user_id = v.user_id
      AND co.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
      AND (
        sqlc.narg(organization_uuids)::uuid[] IS NULL
        OR co.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
      )
  )
  AND (sqlc.narg(plate_normalized)::text IS NULL OR v.plate_normalized LIKE sqlc.narg(plate_normalized) || '%')
  AND (sqlc.narg(vin)::text IS NULL OR v.vin = sqlc.narg(vin))
  AND (
    (sqlc.narg(q)::text IS NULL AND sqlc.narg(q_name)::text IS NULL)
    OR v.plate_normalized LIKE sqlc.narg(q)::text || '%'
    OR v.vin LIKE sqlc.narg(q)::text || '%'
    OR concat_ws(' ', cb.name, cm.name) ILIKE '%' || sqlc.narg(q_name)::text || '%'
  )
  AND (
    sqlc.narg(car_brand_uuids)::uuid[] IS NULL
    OR v.car_brand_id IN (SELECT fb.id FROM car_brands fb WHERE fb.uuid = ANY (sqlc.narg(car_brand_uuids)::uuid[]))
  )
  AND (
    sqlc.narg(car_model_uuids)::uuid[] IS NULL
    OR v.car_model_id IN (SELECT fm.id FROM car_models fm WHERE fm.uuid = ANY (sqlc.narg(car_model_uuids)::uuid[]))
  )
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR v.uuid = ANY (sqlc.narg(uuids)::uuid[]))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'plate' THEN v.plate_normalized END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'plate' THEN v.plate_normalized END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'brand' THEN lower(cb.name) END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'brand' THEN lower(cb.name) END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'model' THEN lower(cm.name) END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'model' THEN lower(cm.name) END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'model_year' THEN v.model_year END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'model_year' THEN v.model_year END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN v.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN v.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN v.id END DESC,
  v.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountScopedVehicles :one
SELECT COUNT(*)::bigint
FROM vehicles v
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.deleted_at IS NULL
  AND v.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(user_id)::bigint IS NULL OR v.user_id = sqlc.narg(user_id))
  AND EXISTS (
    SELECT 1 FROM customer_organizations co
    WHERE co.user_id = v.user_id
      AND co.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR co.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
      AND (
        sqlc.narg(organization_uuids)::uuid[] IS NULL
        OR co.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
      )
  )
  AND (sqlc.narg(plate_normalized)::text IS NULL OR v.plate_normalized LIKE sqlc.narg(plate_normalized) || '%')
  AND (sqlc.narg(vin)::text IS NULL OR v.vin = sqlc.narg(vin))
  AND (
    (sqlc.narg(q)::text IS NULL AND sqlc.narg(q_name)::text IS NULL)
    OR v.plate_normalized LIKE sqlc.narg(q)::text || '%'
    OR v.vin LIKE sqlc.narg(q)::text || '%'
    OR concat_ws(' ', cb.name, cm.name) ILIKE '%' || sqlc.narg(q_name)::text || '%'
  )
  AND (
    sqlc.narg(car_brand_uuids)::uuid[] IS NULL
    OR v.car_brand_id IN (SELECT fb.id FROM car_brands fb WHERE fb.uuid = ANY (sqlc.narg(car_brand_uuids)::uuid[]))
  )
  AND (
    sqlc.narg(car_model_uuids)::uuid[] IS NULL
    OR v.car_model_id IN (SELECT fm.id FROM car_models fm WHERE fm.uuid = ANY (sqlc.narg(car_model_uuids)::uuid[]))
  );

-- TEC-164: Meilisearch customers index. One document per customer linked to
-- at least one organization; anonymized, merged and deleted users never
-- enter the index. organization_ids / brand_ids drive the scope filter.
-- name: ListCustomersForIndex :many
SELECT u.uuid, u.name, u.surname, u.email, u.phone_e164, u.status,
       cp.company_name,
       array_agg(DISTINCT co.organization_id)::bigint[] AS organization_ids,
       array_agg(DISTINCT co.brand_id)::bigint[] AS brand_ids
FROM users u
JOIN customer_organizations co ON co.user_id = u.id
LEFT JOIN customer_profiles cp ON cp.user_id = u.id
WHERE u.deleted_at IS NULL
  AND u.status <> 'anonymized'
  AND u.merged_into_user_id IS NULL
GROUP BY u.id, cp.user_id
ORDER BY u.id;

-- name: GetCustomerForIndex :one
SELECT u.uuid, u.name, u.surname, u.email, u.phone_e164, u.status,
       cp.company_name,
       array_agg(DISTINCT co.organization_id)::bigint[] AS organization_ids,
       array_agg(DISTINCT co.brand_id)::bigint[] AS brand_ids
FROM users u
JOIN customer_organizations co ON co.user_id = u.id
LEFT JOIN customer_profiles cp ON cp.user_id = u.id
WHERE u.uuid = sqlc.arg(uuid)
  AND u.deleted_at IS NULL
  AND u.status <> 'anonymized'
  AND u.merged_into_user_id IS NULL
GROUP BY u.id, cp.user_id;
