-- TEC-259: migrator step 7a (services, items, images, status logs). Written
-- only by cmd/migrator inside a step transaction. Nothing here writes the
-- outbox: the import starts no warranty and sends no notification.

-- name: MigratorOrganizationByUUID :one
SELECT id, uuid, brand_id FROM organizations WHERE uuid = $1;

-- name: MigratorCustomerUserByUUID :one
-- The account a migrated customer maps to; a merged account resolves to the
-- account it was merged into.
SELECT COALESCE(merged_into_user_id, id)::bigint AS id FROM users WHERE uuid = $1;

-- name: MigratorCarModelByUUID :one
SELECT id, car_brand_id FROM car_models WHERE uuid = $1;

-- name: MigratorVehicleByUUID :one
SELECT id, user_id FROM vehicles WHERE uuid = $1;

-- name: MigratorFindVehicle :one
-- A live vehicle of the customer with the VIN, else with the plate (one
-- that has no other VIN). The VIN match wins.
SELECT id, uuid FROM vehicles
WHERE user_id = sqlc.arg(user_id)::bigint AND brand_id = sqlc.arg(brand_id)::bigint AND deleted_at IS NULL
  AND (
        (sqlc.narg(vin)::text IS NOT NULL AND vin = sqlc.narg(vin)::text)
     OR (sqlc.narg(plate_normalized)::text IS NOT NULL
         AND plate_country = sqlc.narg(plate_country)::text
         AND plate_normalized = sqlc.narg(plate_normalized)::text
         AND (vin IS NULL OR sqlc.narg(vin)::text IS NULL))
  )
ORDER BY (vin IS NOT DISTINCT FROM sqlc.narg(vin)::text) DESC, id
LIMIT 1;

-- name: MigratorInsertVehicle :one
INSERT INTO vehicles (uuid, user_id, organization_id, brand_id, car_brand_id, car_model_id, model_year,
                      plate, plate_normalized, plate_country, vin, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(user_id), sqlc.narg(organization_id), sqlc.arg(brand_id), sqlc.narg(car_brand_id),
    sqlc.narg(car_model_id), sqlc.narg(model_year), sqlc.narg(plate), sqlc.narg(plate_normalized),
    sqlc.narg(plate_country), sqlc.narg(vin), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorServiceByUUID :one
SELECT id, uuid, organization_id, brand_id, status FROM services WHERE uuid = $1;

-- name: MigratorServiceNoTaken :one
-- Another service already holds the number.
SELECT EXISTS (
    SELECT 1 FROM services WHERE service_no = sqlc.arg(service_no)::text AND uuid <> sqlc.arg(uuid)::uuid
);

-- name: MigratorInsertService :one
INSERT INTO services (
    uuid, service_no, organization_id, brand_id, customer_user_id, vehicle_id, car_brand_id, car_model_id,
    model_year, plate, plate_country, vin, km, package, notes, status, created_by_user_id,
    completed_at, cancelled_at, review_request_sent_at, created_at, updated_at
)
VALUES (
    sqlc.arg(uuid), sqlc.arg(service_no), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(customer_user_id),
    sqlc.arg(vehicle_id), sqlc.arg(car_brand_id), sqlc.arg(car_model_id), sqlc.narg(model_year), sqlc.narg(plate),
    sqlc.narg(plate_country), sqlc.narg(vin), sqlc.narg(km), sqlc.narg(package), sqlc.narg(notes), sqlc.arg(status),
    sqlc.narg(created_by_user_id), sqlc.narg(completed_at), sqlc.narg(cancelled_at), sqlc.narg(review_request_sent_at),
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW()),
    COALESCE(sqlc.narg(updated_at)::timestamptz, sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateService :exec
-- A changed legacy service. The status is written by MigratorSetServiceStatus.
UPDATE services
SET organization_id = sqlc.arg(organization_id), brand_id = sqlc.arg(brand_id),
    customer_user_id = sqlc.arg(customer_user_id), vehicle_id = sqlc.arg(vehicle_id),
    car_brand_id = sqlc.arg(car_brand_id), car_model_id = sqlc.arg(car_model_id),
    model_year = sqlc.narg(model_year), plate = sqlc.narg(plate), plate_country = sqlc.narg(plate_country),
    vin = sqlc.narg(vin), km = sqlc.narg(km), package = sqlc.narg(package), notes = sqlc.narg(notes),
    created_by_user_id = sqlc.narg(created_by_user_id), review_request_sent_at = sqlc.narg(review_request_sent_at)
WHERE id = sqlc.arg(id);

-- name: MigratorSetServiceStatus :exec
-- The legacy status of a service that is not final yet. A final status is
-- written after the items, which are locked afterwards.
UPDATE services
SET status = sqlc.arg(status), completed_at = sqlc.narg(completed_at), cancelled_at = sqlc.narg(cancelled_at)
WHERE id = sqlc.arg(id) AND status NOT IN ('completed', 'cancelled');

-- name: MigratorServiceUnit :one
-- A unit of the brand with its product and the category's part keys.
SELECT u.id, u.product_id, u.unit_kind, COALESCE(c.available_parts, '[]'::jsonb)::jsonb AS available_parts
FROM units u
JOIN products p ON p.id = u.product_id
LEFT JOIN product_categories c ON c.id = p.category_id
WHERE u.uuid = sqlc.arg(uuid) AND u.brand_id = sqlc.arg(brand_id);

-- name: MigratorServiceItemByUUID :one
SELECT id FROM service_items WHERE uuid = $1;

-- name: MigratorInsertServiceItem :one
INSERT INTO service_items (uuid, service_id, organization_id, brand_id, product_id, unit_id, kind, quantity,
                           applied_parts, notes, created_at, updated_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(service_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id),
    sqlc.arg(unit_id), sqlc.arg(kind), sqlc.narg(quantity), sqlc.arg(applied_parts)::jsonb, sqlc.narg(notes),
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW()),
    COALESCE(sqlc.narg(updated_at)::timestamptz, sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateServiceItem :exec
UPDATE service_items
SET applied_parts = sqlc.arg(applied_parts)::jsonb, notes = sqlc.narg(notes)
WHERE id = sqlc.arg(id);

-- name: MigratorServiceImageByUUID :one
SELECT id FROM service_images WHERE uuid = $1;

-- name: MigratorInsertServiceImage :one
INSERT INTO service_images (uuid, service_id, organization_id, brand_id, storage_key, title, sort_order, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(service_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(storage_key),
    sqlc.narg(title), sqlc.arg(sort_order), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateServiceImage :exec
UPDATE service_images SET title = sqlc.narg(title), sort_order = sqlc.arg(sort_order) WHERE id = sqlc.arg(id);

-- name: MigratorInsertServiceStatusLog :exec
INSERT INTO service_status_logs (service_id, organization_id, brand_id, from_status, to_status, actor_user_id,
                                 actor_org_id, note, metadata, created_at)
VALUES (
    sqlc.arg(service_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(from_status), sqlc.arg(to_status),
    sqlc.narg(actor_user_id), sqlc.narg(actor_org_id), sqlc.narg(note), sqlc.arg(metadata)::jsonb,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
);

-- name: MigratorOrganizationUUIDByID :one
SELECT uuid FROM organizations WHERE id = $1;
