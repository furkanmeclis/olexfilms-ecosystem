-- TEC-191 (F1-06g): panel and portal warranty list, detail and void.
-- Brand-bound (K20). Scope arguments, all optional:
--   org_ids            NULL = whole brand (brand / all scope), else the
--                      organizations in scope (dealer own, distributor subtree);
--   holder_user_id     portal / customer scope: the holder only;
--   service_created_by own / assigned scope: services the caller created.
-- Filters: status, product, vehicle, end_at window (days left), q (warranty
-- public code, service number, product name or plate; q_plate is the
-- normalized plate, geo.NormalizePlate), warranty_uuid (detail).
-- Order: active first by the soonest end, then the rest by the latest end.
-- TEC-377 (DT-BE-7): statuses / organization_uuids are multi-value filters,
-- start_from / start_before and end_from / end_before date windows; the
-- sort keys come from warranty usecase.ListSort (docs/list-contract.md).
-- 'expiry' is the order above (descending reverses it), status sorts by
-- rank (active, expired, void), product / organization by name.

-- name: ListWarrantyRows :many
SELECT w.id, w.uuid, w.public_code, w.organization_id, w.brand_id, w.status, w.item_kind,
       w.start_at, w.end_at, w.expired_at, w.voided_at, w.void_reason, w.created_at,
       s.uuid AS service_uuid, s.service_no, s.plate AS service_plate, s.model_year AS service_model_year,
       p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
       cb.name AS car_brand_name, cm.name AS car_model_name,
       hu.uuid AS holder_uuid, hu.name AS holder_name, hu.surname AS holder_surname,
       hu.status AS holder_status
FROM warranties w
JOIN services s ON s.id = w.service_id
JOIN products p ON p.id = w.product_id
JOIN organizations o ON o.id = w.organization_id
JOIN vehicles v ON v.id = w.vehicle_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
JOIN users hu ON hu.id = w.holder_user_id
WHERE w.brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(holder_user_id)::bigint IS NULL OR w.holder_user_id = sqlc.narg(holder_user_id)::bigint)
  AND (sqlc.narg(service_created_by)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(service_created_by)::bigint)
  AND (sqlc.narg(warranty_uuid)::uuid IS NULL OR w.uuid = sqlc.narg(warranty_uuid)::uuid)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR w.status = ANY (sqlc.narg(statuses)::text[]))
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR w.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (sqlc.narg(start_from)::timestamptz IS NULL OR w.start_at >= sqlc.narg(start_from)::timestamptz)
  AND (sqlc.narg(start_before)::timestamptz IS NULL OR w.start_at < sqlc.narg(start_before)::timestamptz)
  AND (sqlc.narg(end_from)::timestamptz IS NULL OR w.end_at >= sqlc.narg(end_from)::timestamptz)
  AND (sqlc.narg(end_before)::timestamptz IS NULL OR w.end_at < sqlc.narg(end_before)::timestamptz)
  AND (sqlc.narg(product_id)::bigint IS NULL OR w.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR w.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (sqlc.narg(ends_after)::timestamptz IS NULL OR w.end_at > sqlc.narg(ends_after)::timestamptz)
  AND (sqlc.narg(ends_before)::timestamptz IS NULL OR w.end_at <= sqlc.narg(ends_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR w.public_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (sqlc.narg(q_plate)::text IS NOT NULL AND sqlc.narg(q_plate)::text <> '' AND (
              upper(translate(COALESCE(s.plate, ''), ' -._·', '')) LIKE '%' || sqlc.narg(q_plate)::text || '%'
              OR COALESCE(v.plate_normalized, '') LIKE '%' || sqlc.narg(q_plate)::text || '%')))
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR w.uuid = ANY (sqlc.narg(uuids)::uuid[]))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'expiry' THEN
    CASE WHEN w.status = 'active' THEN w.end_at END
  END ASC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'expiry' THEN w.end_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'expiry' THEN
    CASE WHEN w.status = 'active' THEN w.end_at END
  END DESC NULLS FIRST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'expiry' THEN w.end_at END ASC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'public_code' THEN w.public_code::text
      WHEN 'service_no' THEN s.service_no::text
      WHEN 'product' THEN p.name::text
      WHEN 'organization' THEN o.name::text
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'public_code' THEN w.public_code::text
      WHEN 'service_no' THEN s.service_no::text
      WHEN 'product' THEN p.name::text
      WHEN 'organization' THEN o.name::text
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE w.status WHEN 'active' THEN 0 WHEN 'expired' THEN 1 WHEN 'void' THEN 2 ELSE 3 END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE w.status WHEN 'active' THEN 0 WHEN 'expired' THEN 1 WHEN 'void' THEN 2 ELSE 3 END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'end_at' THEN w.end_at WHEN 'start_at' THEN w.start_at WHEN 'created_at' THEN w.created_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'end_at' THEN w.end_at WHEN 'start_at' THEN w.start_at WHEN 'created_at' THEN w.created_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN w.id END DESC,
  w.id ASC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountWarrantyRows :one
SELECT COUNT(*)
FROM warranties w
JOIN services s ON s.id = w.service_id
JOIN products p ON p.id = w.product_id
JOIN vehicles v ON v.id = w.vehicle_id
WHERE w.brand_id = sqlc.arg(brand_id)::bigint
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(holder_user_id)::bigint IS NULL OR w.holder_user_id = sqlc.narg(holder_user_id)::bigint)
  AND (sqlc.narg(service_created_by)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(service_created_by)::bigint)
  AND (sqlc.narg(warranty_uuid)::uuid IS NULL OR w.uuid = sqlc.narg(warranty_uuid)::uuid)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR w.status = ANY (sqlc.narg(statuses)::text[]))
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR w.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (sqlc.narg(start_from)::timestamptz IS NULL OR w.start_at >= sqlc.narg(start_from)::timestamptz)
  AND (sqlc.narg(start_before)::timestamptz IS NULL OR w.start_at < sqlc.narg(start_before)::timestamptz)
  AND (sqlc.narg(end_from)::timestamptz IS NULL OR w.end_at >= sqlc.narg(end_from)::timestamptz)
  AND (sqlc.narg(end_before)::timestamptz IS NULL OR w.end_at < sqlc.narg(end_before)::timestamptz)
  AND (sqlc.narg(product_id)::bigint IS NULL OR w.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR w.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (sqlc.narg(ends_after)::timestamptz IS NULL OR w.end_at > sqlc.narg(ends_after)::timestamptz)
  AND (sqlc.narg(ends_before)::timestamptz IS NULL OR w.end_at <= sqlc.narg(ends_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR w.public_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (sqlc.narg(q_plate)::text IS NOT NULL AND sqlc.narg(q_plate)::text <> '' AND (
              upper(translate(COALESCE(s.plate, ''), ' -._·', '')) LIKE '%' || sqlc.narg(q_plate)::text || '%'
              OR COALESCE(v.plate_normalized, '') LIKE '%' || sqlc.narg(q_plate)::text || '%')));

-- Center void (warranties.void, step-up). An expired warranty may be voided
-- too: chk_warranties_expired ties expired_at to status 'expired', so the
-- expiry stamp is cleared with the status change (the voided_at stamp and
-- the activity log keep the history).
-- name: VoidWarrantyWithReason :one
UPDATE warranties
SET status = 'void',
    voided_at = NOW(),
    expired_at = NULL,
    voided_by_user_id = sqlc.narg(actor_user_id),
    void_reason = sqlc.arg(void_reason)::text
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status IN ('active', 'expired')
RETURNING *;
