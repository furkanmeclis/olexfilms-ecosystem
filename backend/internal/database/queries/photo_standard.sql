-- TEC-498 (F5-07a): vehicle intake photo standard.

-- name: CreatePhotoAngle :one
INSERT INTO photo_angles (
    organization_id, brand_id, key, name, hint, example_storage_key, required, sort_order, active
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(key), sqlc.arg(name)::jsonb,
    sqlc.arg(hint)::jsonb, sqlc.narg(example_storage_key), sqlc.arg(required), sqlc.arg(sort_order), sqlc.arg(active)
)
RETURNING *;

-- name: ListPhotoAnglesByBrand :many
SELECT * FROM photo_angles
WHERE brand_id = sqlc.arg(brand_id)
ORDER BY sort_order ASC, id ASC;

-- name: GetPhotoAngleByUUID :one
SELECT * FROM photo_angles
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetPhotoAngleByKey :one
SELECT * FROM photo_angles
WHERE key = sqlc.arg(key) AND brand_id = sqlc.arg(brand_id);

-- name: UpdatePhotoAngle :one
UPDATE photo_angles
SET name = sqlc.arg(name)::jsonb,
    hint = sqlc.arg(hint)::jsonb,
    example_storage_key = sqlc.narg(example_storage_key),
    required = sqlc.arg(required),
    sort_order = sqlc.arg(sort_order),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeletePhotoAngle :execrows
DELETE FROM photo_angles
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: UpsertPhotoAngleOverride :one
INSERT INTO photo_angle_overrides (
    organization_id, brand_id, angle_id, required, hidden, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(angle_id),
    sqlc.arg(required), sqlc.arg(hidden), sqlc.narg(created_by_user_id)
)
ON CONFLICT (organization_id, angle_id) DO UPDATE SET
    required = EXCLUDED.required,
    hidden = EXCLUDED.hidden,
    created_by_user_id = EXCLUDED.created_by_user_id
RETURNING *;

-- name: DeletePhotoAngleOverridesForOrg :exec
DELETE FROM photo_angle_overrides
WHERE organization_id = sqlc.arg(organization_id) AND brand_id = sqlc.arg(brand_id);

-- name: ListPhotoAngleOverridesForOrg :many
SELECT o.*, a.key AS angle_key
FROM photo_angle_overrides o
JOIN photo_angles a ON a.id = o.angle_id
WHERE o.organization_id = sqlc.arg(organization_id)
  AND o.brand_id = sqlc.arg(brand_id)
ORDER BY a.sort_order ASC, a.id ASC;

-- name: ListResolvedPhotoAngles :many
SELECT
    a.id, a.uuid, a.key, a.name, a.hint, a.example_storage_key, a.sort_order, a.active,
    COALESCE(dealer.required, distributor.required, center.required, a.required)::boolean AS resolved_required,
    COALESCE(dealer.hidden, distributor.hidden, center.hidden, false)::boolean AS resolved_hidden
FROM photo_angles a
LEFT JOIN photo_angle_overrides center
    ON center.angle_id = a.id AND center.organization_id = a.organization_id
LEFT JOIN photo_angle_overrides distributor
    ON distributor.angle_id = a.id AND distributor.organization_id = sqlc.narg(distributor_org_id)
LEFT JOIN photo_angle_overrides dealer
    ON dealer.angle_id = a.id AND dealer.organization_id = sqlc.arg(service_org_id)
WHERE a.brand_id = sqlc.arg(brand_id)
  AND a.active
ORDER BY a.sort_order ASC, a.id ASC;

-- name: ListActiveIntakePhotosForService :many
SELECT * FROM intake_photos
WHERE service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC;

-- name: SoftDeleteActiveIntakePhoto :one
UPDATE intake_photos
SET deleted_at = NOW(), deleted_by = sqlc.narg(deleted_by)
WHERE service_id = sqlc.arg(service_id)
  AND angle_id = sqlc.arg(angle_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: CreateIntakePhoto :one
INSERT INTO intake_photos (
    service_id, organization_id, brand_id, angle_id, storage_key, mime, size, sha256,
    width, height, exif_taken_at, exif_lat, exif_lng, exif_device, uploaded_by
)
SELECT s.id, s.organization_id, s.brand_id, sqlc.arg(angle_id), sqlc.arg(storage_key),
       sqlc.arg(mime), sqlc.arg(size), sqlc.arg(sha256), sqlc.narg(width), sqlc.narg(height),
       sqlc.narg(exif_taken_at), sqlc.narg(exif_lat), sqlc.narg(exif_lng), sqlc.narg(exif_device),
       sqlc.narg(uploaded_by)
FROM services s
WHERE s.id = sqlc.arg(service_id)
RETURNING *;

-- name: GetActiveIntakePhotoByUUID :one
SELECT * FROM intake_photos
WHERE uuid = sqlc.arg(uuid)
  AND service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL;

-- name: GetActiveIntakePhotoByAngle :one
SELECT * FROM intake_photos
WHERE service_id = sqlc.arg(service_id)
  AND angle_id = sqlc.arg(angle_id)
  AND deleted_at IS NULL;

-- name: HasExecutedServiceContract :one
SELECT EXISTS (
    SELECT 1
    FROM services s
    JOIN contract_instances ci ON ci.id = s.contract_id
    WHERE s.id = sqlc.arg(service_id)
      AND ci.status = 'executed'
)::boolean AS exists;

-- TEC-499 (F5-07b): KVKK anonymization of a customer clears the EXIF
-- location and device of the intake photos of their services (the photos
-- and capture time stay as service evidence).
-- name: ClearCustomerIntakePhotoEXIF :execrows
UPDATE intake_photos ip
SET exif_lat = NULL, exif_lng = NULL, exif_device = NULL
FROM services s
WHERE s.id = ip.service_id
  AND s.customer_user_id = sqlc.arg(customer_user_id)
  AND (ip.exif_lat IS NOT NULL OR ip.exif_lng IS NOT NULL OR ip.exif_device IS NOT NULL);
