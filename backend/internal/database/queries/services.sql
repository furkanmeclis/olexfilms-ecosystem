-- TEC-178 (F1-05a): services, service items, images and status logs
-- (migration 000050). Every read is brand-bound (K20, decision 2: the
-- brand is the service organization's brand). Transitions lock the row
-- first (Lock* ... FOR UPDATE) in the use case transaction.

-- ---------------------------------------------------------------------------
-- Services.

-- name: CreateService :one
INSERT INTO services (
    service_no, organization_id, brand_id, customer_user_id, vehicle_id,
    car_brand_id, car_model_id, model_year, plate, plate_country, vin, km,
    package, notes, has_measurement, measurement_result_id, contract_id,
    status, created_by_user_id, updated_by_user_id
)
VALUES (
    sqlc.arg(service_no), sqlc.arg(organization_id), sqlc.arg(brand_id),
    sqlc.arg(customer_user_id), sqlc.arg(vehicle_id),
    sqlc.arg(car_brand_id), sqlc.arg(car_model_id), sqlc.narg(model_year),
    sqlc.narg(plate), sqlc.narg(plate_country), sqlc.narg(vin), sqlc.narg(km),
    sqlc.narg(package), sqlc.narg(notes), sqlc.arg(has_measurement),
    sqlc.narg(measurement_result_id), sqlc.narg(contract_id),
    sqlc.arg(status), sqlc.narg(created_by_user_id), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetService :one
SELECT * FROM services
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetServiceByUUID :one
SELECT * FROM services
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- Public warranty / PDF lookup by number (unique across brands).
-- name: GetServiceByNo :one
SELECT * FROM services
WHERE service_no = sqlc.arg(service_no);

-- name: ServiceNoExists :one
SELECT EXISTS (SELECT 1 FROM services WHERE service_no = sqlc.arg(service_no))::boolean AS exists;

-- name: LockService :one
SELECT * FROM services
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: LockServiceByUUID :one
SELECT * FROM services
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- Edits the form fields (wizard steps 1, 2 and 4). The caller has locked
-- the row and checked the form lock (completed / cancelled: center only).
-- name: UpdateService :one
UPDATE services
SET customer_user_id      = sqlc.arg(customer_user_id),
    vehicle_id            = sqlc.arg(vehicle_id),
    car_brand_id          = sqlc.arg(car_brand_id),
    car_model_id          = sqlc.arg(car_model_id),
    model_year            = sqlc.narg(model_year),
    plate                 = sqlc.narg(plate),
    plate_country         = sqlc.narg(plate_country),
    vin                   = sqlc.narg(vin),
    km                    = sqlc.narg(km),
    package               = sqlc.narg(package),
    notes                 = sqlc.narg(notes),
    has_measurement       = sqlc.arg(has_measurement),
    measurement_result_id = sqlc.narg(measurement_result_id),
    contract_id           = sqlc.narg(contract_id),
    updated_by_user_id    = sqlc.narg(updated_by_user_id)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- Moves the service to status (not completed / cancelled, which have their
-- own queries). The caller has locked the row and validated the transition.
-- name: UpdateServiceStatus :one
UPDATE services
SET status = sqlc.arg(status)::text,
    updated_by_user_id = sqlc.narg(actor_user_id)
WHERE id = sqlc.arg(id)
  AND sqlc.arg(status)::text NOT IN ('completed', 'cancelled')
RETURNING *;

-- name: CompleteService :one
UPDATE services
SET status = 'completed',
    completed_at = NOW(),
    completed_by_user_id = sqlc.narg(actor_user_id),
    updated_by_user_id = sqlc.narg(actor_user_id)
WHERE id = sqlc.arg(id) AND status NOT IN ('completed', 'cancelled')
RETURNING *;

-- name: CancelService :one
UPDATE services
SET status = 'cancelled',
    cancelled_at = NOW(),
    cancelled_by_user_id = sqlc.narg(actor_user_id),
    updated_by_user_id = sqlc.narg(actor_user_id),
    cancel_reason = sqlc.narg(cancel_reason)
WHERE id = sqlc.arg(id) AND status NOT IN ('completed', 'cancelled')
RETURNING *;

-- name: SetServiceReviewRequestSent :one
UPDATE services
SET review_request_sent_at = COALESCE(review_request_sent_at, NOW())
WHERE id = sqlc.arg(id)
RETURNING *;

-- Draft deletion (items, images and logs must be gone first).
-- name: DeleteDraftService :execrows
DELETE FROM services
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status = 'draft';

-- Scope list: org_ids NULL = whole brand (brand/all scope); created_by for
-- scope own, customer_user_id for scope customer (portal).
-- name: ListServicesInScope :many
SELECT * FROM services
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR created_by_user_id = sqlc.narg(created_by_user_id))
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR customer_user_id = sqlc.narg(customer_user_id))
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (
    sqlc.narg(q)::text IS NULL
    OR service_no ILIKE '%' || sqlc.narg(q) || '%'
    OR plate ILIKE '%' || sqlc.narg(q) || '%'
    OR vin ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountServicesInScope :one
SELECT COUNT(*) FROM services
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR created_by_user_id = sqlc.narg(created_by_user_id))
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR customer_user_id = sqlc.narg(customer_user_id))
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (
    sqlc.narg(q)::text IS NULL
    OR service_no ILIKE '%' || sqlc.narg(q) || '%'
    OR plate ILIKE '%' || sqlc.narg(q) || '%'
    OR vin ILIKE '%' || sqlc.narg(q) || '%'
  );

-- Services of a customer across brands' organizations in scope (portal and
-- customer detail).
-- name: ListServicesByCustomer :many
SELECT * FROM services
WHERE customer_user_id = sqlc.arg(customer_user_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- ---------------------------------------------------------------------------
-- Service items. Locked by trigger once the service is completed or
-- cancelled.

-- name: CreateServiceItem :one
INSERT INTO service_items (
    service_id, organization_id, brand_id, product_id, unit_id, kind,
    quantity, meters, applied_parts, notes
)
SELECT s.id, s.organization_id, s.brand_id, sqlc.arg(product_id), sqlc.arg(unit_id), sqlc.arg(kind),
       sqlc.narg(quantity), sqlc.narg(meters), sqlc.arg(applied_parts), sqlc.narg(notes)
FROM services s
WHERE s.id = sqlc.arg(service_id)
RETURNING *;

-- name: GetServiceItem :one
SELECT * FROM service_items
WHERE id = sqlc.arg(id) AND service_id = sqlc.arg(service_id);

-- name: GetServiceItemByUUID :one
SELECT * FROM service_items
WHERE uuid = sqlc.arg(uuid) AND service_id = sqlc.arg(service_id);

-- name: ListServiceItems :many
SELECT * FROM service_items
WHERE service_id = sqlc.arg(service_id)
ORDER BY id;

-- Completion locks the lines in id order (deadlock-free with concurrent
-- completions sharing a unit).
-- name: LockServiceItems :many
SELECT * FROM service_items
WHERE service_id = sqlc.arg(service_id)
ORDER BY id
FOR UPDATE;

-- name: UpdateServiceItem :one
UPDATE service_items
SET kind = sqlc.arg(kind),
    quantity = sqlc.narg(quantity),
    meters = sqlc.narg(meters),
    applied_parts = sqlc.arg(applied_parts),
    notes = sqlc.narg(notes)
WHERE id = sqlc.arg(id) AND service_id = sqlc.arg(service_id)
RETURNING *;

-- Written in the completion transaction before the status flips.
-- name: SetServiceItemMovement :one
UPDATE service_items
SET stock_movement_id = sqlc.arg(stock_movement_id)
WHERE id = sqlc.arg(id) AND stock_movement_id IS NULL
RETURNING *;

-- name: DeleteServiceItem :execrows
DELETE FROM service_items
WHERE id = sqlc.arg(id) AND service_id = sqlc.arg(service_id);

-- name: DeleteServiceItemsByService :execrows
DELETE FROM service_items
WHERE service_id = sqlc.arg(service_id);

-- Other open services holding the same unit (draft check; the ledger has
-- the final word on completion).
-- name: ListOpenServicesByUnit :many
SELECT s.id, s.uuid, s.service_no, s.organization_id, s.status, i.id AS item_id, i.kind
FROM service_items i
JOIN services s ON s.id = i.service_id
WHERE i.unit_id = sqlc.arg(unit_id)
  AND s.status NOT IN ('completed', 'cancelled')
  AND (sqlc.narg(exclude_service_id)::bigint IS NULL OR s.id <> sqlc.narg(exclude_service_id))
ORDER BY s.id;

-- ---------------------------------------------------------------------------
-- Images.

-- name: CreateServiceImage :one
INSERT INTO service_images (
    service_id, organization_id, brand_id, storage_key, title, sort_order, uploaded_by_user_id
)
SELECT s.id, s.organization_id, s.brand_id, sqlc.arg(storage_key), sqlc.narg(title),
       sqlc.arg(sort_order), sqlc.narg(uploaded_by_user_id)
FROM services s
WHERE s.id = sqlc.arg(service_id)
RETURNING *;

-- name: GetServiceImage :one
SELECT * FROM service_images
WHERE id = sqlc.arg(id) AND service_id = sqlc.arg(service_id);

-- name: ListServiceImages :many
SELECT * FROM service_images
WHERE service_id = sqlc.arg(service_id)
ORDER BY sort_order, id;

-- name: UpdateServiceImage :one
UPDATE service_images
SET title = sqlc.narg(title), sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND service_id = sqlc.arg(service_id)
RETURNING *;

-- name: DeleteServiceImage :one
DELETE FROM service_images
WHERE id = sqlc.arg(id) AND service_id = sqlc.arg(service_id)
RETURNING *;

-- ---------------------------------------------------------------------------
-- Status log (append-only).

-- name: InsertServiceStatusLog :one
INSERT INTO service_status_logs (
    service_id, organization_id, brand_id, from_status, to_status,
    actor_user_id, actor_org_id, note, metadata
)
SELECT s.id, s.organization_id, s.brand_id, sqlc.narg(from_status), sqlc.arg(to_status),
       sqlc.narg(actor_user_id), sqlc.narg(actor_org_id), sqlc.narg(note), sqlc.arg(metadata)
FROM services s
WHERE s.id = sqlc.arg(service_id)
RETURNING *;

-- name: ListServiceStatusLogs :many
SELECT * FROM service_status_logs
WHERE service_id = sqlc.arg(service_id)
ORDER BY created_at, id;
