-- TEC-209: Meilisearch services / warranties / vehicles indexes. One
-- document per record; the list endpoints filter the index on the caller's
-- scope (brand + organizations, own / assigned, holder) and reload the hits
-- from Postgres with the same scope, so the index is never the only access
-- check. The customer columns feed the document only while the customer
-- is not anonymized (K19): the adapter drops them otherwise.

-- name: ListServicesForIndex :many
SELECT s.uuid, s.service_no, s.organization_id, s.brand_id, s.status,
       s.plate, s.vin, s.package, s.customer_user_id, s.vehicle_id, s.created_by_user_id,
       cu.status AS customer_status, cu.name AS customer_name, cu.surname AS customer_surname,
       cu.phone_e164 AS customer_phone,
       cb.name AS car_brand_name, cm.name AS car_model_name
FROM services s
JOIN users cu ON cu.id = s.customer_user_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
ORDER BY s.id;

-- name: GetServiceForIndex :one
SELECT s.uuid, s.service_no, s.organization_id, s.brand_id, s.status,
       s.plate, s.vin, s.package, s.customer_user_id, s.vehicle_id, s.created_by_user_id,
       cu.status AS customer_status, cu.name AS customer_name, cu.surname AS customer_surname,
       cu.phone_e164 AS customer_phone,
       cb.name AS car_brand_name, cm.name AS car_model_name
FROM services s
JOIN users cu ON cu.id = s.customer_user_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE s.uuid = sqlc.arg(uuid);

-- name: ListWarrantiesForIndex :many
SELECT w.uuid, w.public_code, w.organization_id, w.brand_id, w.status, w.item_kind,
       w.holder_user_id, w.vehicle_id, w.product_id, w.end_at,
       s.service_no, s.created_by_user_id AS service_created_by, s.plate AS service_plate,
       v.plate AS vehicle_plate, v.plate_normalized AS vehicle_plate_normalized, v.vin AS vehicle_vin,
       p.sku AS product_sku, p.name AS product_name,
       hu.status AS holder_status, hu.name AS holder_name, hu.surname AS holder_surname,
       hu.phone_e164 AS holder_phone
FROM warranties w
JOIN services s ON s.id = w.service_id
JOIN vehicles v ON v.id = w.vehicle_id
JOIN products p ON p.id = w.product_id
JOIN users hu ON hu.id = w.holder_user_id
ORDER BY w.id;

-- name: GetWarrantyForIndex :one
SELECT w.uuid, w.public_code, w.organization_id, w.brand_id, w.status, w.item_kind,
       w.holder_user_id, w.vehicle_id, w.product_id, w.end_at,
       s.service_no, s.created_by_user_id AS service_created_by, s.plate AS service_plate,
       v.plate AS vehicle_plate, v.plate_normalized AS vehicle_plate_normalized, v.vin AS vehicle_vin,
       p.sku AS product_sku, p.name AS product_name,
       hu.status AS holder_status, hu.name AS holder_name, hu.surname AS holder_surname,
       hu.phone_e164 AS holder_phone
FROM warranties w
JOIN services s ON s.id = w.service_id
JOIN vehicles v ON v.id = w.vehicle_id
JOIN products p ON p.id = w.product_id
JOIN users hu ON hu.id = w.holder_user_id
WHERE w.uuid = sqlc.arg(uuid);

-- Vehicles: organization_ids are the owner's customer_organizations of the
-- vehicle brand (the same EXISTS the list applies). Deleted vehicles and
-- vehicles whose owner is anonymized, merged or deleted are not indexed.
-- name: ListVehiclesForIndex :many
SELECT v.uuid, v.user_id, v.brand_id, v.plate, v.plate_normalized, v.plate_country, v.vin, v.model_year,
       cb.name AS car_brand_name, cm.name AS car_model_name,
       u.name AS owner_name, u.surname AS owner_surname, u.phone_e164 AS owner_phone,
       COALESCE((SELECT array_agg(DISTINCT co.organization_id) FROM customer_organizations co
                 WHERE co.user_id = v.user_id AND co.brand_id = v.brand_id), '{}')::bigint[] AS organization_ids
FROM vehicles v
JOIN users u ON u.id = v.user_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.deleted_at IS NULL
  AND u.deleted_at IS NULL
  AND u.status <> 'anonymized'
  AND u.merged_into_user_id IS NULL
ORDER BY v.id;

-- name: GetVehicleForIndex :one
SELECT v.uuid, v.user_id, v.brand_id, v.plate, v.plate_normalized, v.plate_country, v.vin, v.model_year,
       cb.name AS car_brand_name, cm.name AS car_model_name,
       u.name AS owner_name, u.surname AS owner_surname, u.phone_e164 AS owner_phone,
       COALESCE((SELECT array_agg(DISTINCT co.organization_id) FROM customer_organizations co
                 WHERE co.user_id = v.user_id AND co.brand_id = v.brand_id), '{}')::bigint[] AS organization_ids
FROM vehicles v
JOIN users u ON u.id = v.user_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL
  AND u.deleted_at IS NULL
  AND u.status <> 'anonymized'
  AND u.merged_into_user_id IS NULL;

-- Records of one customer: refreshed after anonymization (the documents
-- lose the personal data) and after an ownership transfer.
-- name: ListSearchUuidsByCustomer :one
SELECT COALESCE((SELECT array_agg(s.uuid) FROM services s WHERE s.customer_user_id = u.id), '{}')::uuid[] AS service_uuids,
       COALESCE((SELECT array_agg(v.uuid) FROM vehicles v WHERE v.user_id = u.id), '{}')::uuid[] AS vehicle_uuids,
       COALESCE((SELECT array_agg(w.uuid) FROM warranties w WHERE w.holder_user_id = u.id), '{}')::uuid[] AS warranty_uuids
FROM users u
WHERE u.uuid = sqlc.arg(uuid);

-- name: ListSearchUuidsByUserID :one
SELECT COALESCE((SELECT array_agg(s.uuid) FROM services s WHERE s.customer_user_id = sqlc.arg(user_id)::bigint), '{}')::uuid[] AS service_uuids,
       COALESCE((SELECT array_agg(v.uuid) FROM vehicles v WHERE v.user_id = sqlc.arg(user_id)::bigint), '{}')::uuid[] AS vehicle_uuids,
       COALESCE((SELECT array_agg(w.uuid) FROM warranties w WHERE w.holder_user_id = sqlc.arg(user_id)::bigint), '{}')::uuid[] AS warranty_uuids;
