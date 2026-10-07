-- TEC-472 (F5-02a): fleets. A fleet is an organization of type 'fleet'
-- with a fleet_profiles row; dealers reach it through fleet_dealer_links.

-- name: CreateFleetProfile :one
INSERT INTO fleet_profiles (
    organization_id, brand_id, tax_number, tax_office, legal_name,
    contact_name, contact_phone, billing_email, report_frequency, report_locale,
    primary_user_id, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(tax_number), sqlc.narg(tax_office),
    sqlc.arg(legal_name), sqlc.narg(contact_name), sqlc.narg(contact_phone), sqlc.narg(billing_email),
    sqlc.arg(report_frequency), sqlc.arg(report_locale),
    sqlc.narg(primary_user_id), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetFleetProfileByOrg :one
SELECT * FROM fleet_profiles WHERE organization_id = sqlc.arg(organization_id);

-- name: UpdateFleetProfile :one
-- NULL keeps a column; the optional text columns are cleared by the
-- matching clear_* flag.
UPDATE fleet_profiles
SET tax_office = CASE WHEN sqlc.arg(clear_tax_office)::bool THEN NULL
                      ELSE COALESCE(sqlc.narg(tax_office), tax_office) END,
    legal_name = COALESCE(sqlc.narg(legal_name), legal_name),
    contact_name = CASE WHEN sqlc.arg(clear_contact_name)::bool THEN NULL
                        ELSE COALESCE(sqlc.narg(contact_name), contact_name) END,
    contact_phone = CASE WHEN sqlc.arg(clear_contact_phone)::bool THEN NULL
                         ELSE COALESCE(sqlc.narg(contact_phone), contact_phone) END,
    billing_email = CASE WHEN sqlc.arg(clear_billing_email)::bool THEN NULL
                         ELSE COALESCE(sqlc.narg(billing_email), billing_email) END,
    report_frequency = COALESCE(sqlc.narg(report_frequency), report_frequency),
    report_locale = COALESCE(sqlc.narg(report_locale), report_locale),
    primary_user_id = COALESCE(sqlc.narg(primary_user_id), primary_user_id)
WHERE organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: FindFleetByTaxNumber :one
-- Exact VKN/TCKN match within the brand (a second dealer finds the fleet
-- and requests a link instead of opening a duplicate).
SELECT sqlc.embed(o), sqlc.embed(fp)
FROM fleet_profiles fp
JOIN organizations o ON o.id = fp.organization_id AND o.deleted_at IS NULL
WHERE fp.brand_id = sqlc.arg(brand_id) AND fp.tax_number = sqlc.arg(tax_number);

-- name: GetFleetByUUID :one
SELECT sqlc.embed(o), sqlc.embed(fp)
FROM organizations o
JOIN fleet_profiles fp ON fp.organization_id = o.id
WHERE o.uuid = sqlc.arg(uuid) AND o.type = 'fleet' AND o.deleted_at IS NULL;

-- name: ListDealerFleets :many
-- Fleets linked to the dealers in scope, one row per link. dealer_org_ids
-- NULL: every dealer of the brand (brand/all scope). Sort: docs/list-contract.md,
-- keys name | vehicle_count | last_service_at | created_at (the link's),
-- default name. last_service_at is the last completed service of a fleet
-- vehicle at the linked dealer.
WITH fl AS (
    SELECT l.id AS link_id, l.uuid AS link_uuid, l.status AS link_status,
           l.started_at, l.ended_at, l.created_at,
           l.cari_account_id, l.dealer_org_id,
           d.uuid AS dealer_uuid, d.name AS dealer_name,
           f.id AS fleet_org_id, f.uuid AS fleet_uuid, f.name, f.status AS fleet_status,
           fp.tax_number, fp.legal_name,
           (SELECT COUNT(*) FROM vehicles v
            WHERE v.fleet_org_id = f.id AND v.deleted_at IS NULL)::bigint AS vehicle_count,
           (SELECT MAX(s.completed_at) FROM services s
            JOIN vehicles v ON v.id = s.vehicle_id
            WHERE v.fleet_org_id = f.id AND s.organization_id = l.dealer_org_id
              AND s.status = 'completed')::timestamptz AS last_service_at
    FROM fleet_dealer_links l
    JOIN organizations f ON f.id = l.fleet_org_id AND f.deleted_at IS NULL
    JOIN organizations d ON d.id = l.dealer_org_id
    JOIN fleet_profiles fp ON fp.organization_id = f.id
    WHERE l.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(dealer_org_ids)::bigint[] IS NULL OR l.dealer_org_id = ANY (sqlc.narg(dealer_org_ids)::bigint[]))
      AND (
        COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
        OR l.status = ANY (sqlc.narg(statuses)::text[])
      )
      AND (
        sqlc.narg(q)::text IS NULL
        OR f.name ILIKE '%' || sqlc.narg(q) || '%'
        OR fp.legal_name ILIKE '%' || sqlc.narg(q) || '%'
        OR fp.tax_number LIKE sqlc.narg(q) || '%'
      )
)
SELECT * FROM fl
WHERE (sqlc.narg(vehicle_count_min)::bigint IS NULL OR fl.vehicle_count >= sqlc.narg(vehicle_count_min))
  AND (sqlc.narg(vehicle_count_max)::bigint IS NULL OR fl.vehicle_count <= sqlc.narg(vehicle_count_max))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN fl.name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN fl.name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'vehicle_count' THEN fl.vehicle_count END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'vehicle_count' THEN fl.vehicle_count END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN fl.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN fl.created_at END DESC,
  -- Nullable column: its own pair so NULLS LAST does not affect the others.
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_service_at' THEN fl.last_service_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_service_at' THEN fl.last_service_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN fl.link_id END DESC,
  fl.link_id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDealerFleets :one
-- Same filter block as ListDealerFleets.
SELECT COUNT(*)::bigint
FROM fleet_dealer_links l
JOIN organizations f ON f.id = l.fleet_org_id AND f.deleted_at IS NULL
JOIN fleet_profiles fp ON fp.organization_id = f.id
WHERE l.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(dealer_org_ids)::bigint[] IS NULL OR l.dealer_org_id = ANY (sqlc.narg(dealer_org_ids)::bigint[]))
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR l.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR f.name ILIKE '%' || sqlc.narg(q) || '%'
    OR fp.legal_name ILIKE '%' || sqlc.narg(q) || '%'
    OR fp.tax_number LIKE sqlc.narg(q) || '%'
  )
  AND (
    (sqlc.narg(vehicle_count_min)::bigint IS NULL AND sqlc.narg(vehicle_count_max)::bigint IS NULL)
    OR (
      SELECT COUNT(*) FROM vehicles v WHERE v.fleet_org_id = f.id AND v.deleted_at IS NULL
    ) BETWEEN COALESCE(sqlc.narg(vehicle_count_min)::bigint, 0)
          AND COALESCE(sqlc.narg(vehicle_count_max)::bigint, 9223372036854775807)
  );

-- name: ListFleetVehicles :many
-- Vehicles of a fleet. service_org_ids limits last_service_at and
-- active_warranty_count to services of those organizations (a dealer sees
-- its own work); NULL counts every service (portal, center). Sort keys
-- plate | car_brand | last_service_at | active_warranty_count | created_at,
-- default plate.
WITH fv AS (
    SELECT v.id, v.uuid, v.user_id, v.plate, v.plate_country, v.vin, v.model_year, v.created_at,
           cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
           cm.uuid AS car_model_uuid, cm.name AS car_model_name,
           (SELECT MAX(s.completed_at) FROM services s
            WHERE s.vehicle_id = v.id AND s.status = 'completed'
              AND (sqlc.narg(service_org_ids)::bigint[] IS NULL
                   OR s.organization_id = ANY (sqlc.narg(service_org_ids)::bigint[])))::timestamptz AS last_service_at,
           (SELECT COUNT(*) FROM warranties w
            WHERE w.vehicle_id = v.id AND w.status = 'active' AND w.end_at > NOW()
              AND (sqlc.narg(service_org_ids)::bigint[] IS NULL
                   OR w.organization_id = ANY (sqlc.narg(service_org_ids)::bigint[])))::bigint AS active_warranty_count
    FROM vehicles v
    LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
    LEFT JOIN car_models cm ON cm.id = v.car_model_id
    WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND v.deleted_at IS NULL
      AND (
        sqlc.narg(q)::text IS NULL
        OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
        OR v.plate_normalized LIKE sqlc.narg(q_plate)::text || '%'
        OR v.vin ILIKE sqlc.narg(q) || '%'
        OR cb.name ILIKE '%' || sqlc.narg(q) || '%'
        OR cm.name ILIKE '%' || sqlc.narg(q) || '%'
      )
)
SELECT * FROM fv
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'plate' THEN fv.plate END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'plate' THEN fv.plate END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'car_brand' THEN fv.car_brand_name END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'car_brand' THEN fv.car_brand_name END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_service_at' THEN fv.last_service_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_service_at' THEN fv.last_service_at END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'active_warranty_count' THEN fv.active_warranty_count END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'active_warranty_count' THEN fv.active_warranty_count END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN fv.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN fv.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN fv.id END DESC,
  fv.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetVehicles :one
-- Same filter block as ListFleetVehicles.
SELECT COUNT(*)::bigint
FROM vehicles v
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND v.deleted_at IS NULL
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_normalized LIKE sqlc.narg(q_plate)::text || '%'
    OR v.vin ILIKE sqlc.narg(q) || '%'
    OR cb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR cm.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: SetVehicleFleet :one
-- Assigns a vehicle to a fleet (NULL: removes it).
UPDATE vehicles
SET fleet_org_id = sqlc.narg(fleet_org_id)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: CreateFleetDealerLink :one
INSERT INTO fleet_dealer_links (
    fleet_org_id, dealer_org_id, brand_id, status, created_by_org_id, created_by_user_id,
    cari_account_id, started_at
) VALUES (
    sqlc.arg(fleet_org_id), sqlc.arg(dealer_org_id), sqlc.arg(brand_id), sqlc.arg(status)::text,
    sqlc.arg(created_by_org_id), sqlc.narg(created_by_user_id), sqlc.narg(cari_account_id),
    CASE WHEN sqlc.arg(status)::text = 'active' THEN NOW() END
)
RETURNING *;

-- name: GetFleetDealerLinkByUUID :one
SELECT * FROM fleet_dealer_links WHERE uuid = sqlc.arg(uuid);

-- name: GetOpenFleetDealerLink :one
-- The pending or active link of (fleet, dealer), if any.
SELECT * FROM fleet_dealer_links
WHERE fleet_org_id = sqlc.arg(fleet_org_id) AND dealer_org_id = sqlc.arg(dealer_org_id)
  AND status <> 'ended';

-- name: ListFleetDealerLinks :many
-- Links of a fleet with the dealer names (fleet card, portal).
SELECT sqlc.embed(l), d.uuid AS dealer_uuid, d.name AS dealer_name, d.type AS dealer_type
FROM fleet_dealer_links l
JOIN organizations d ON d.id = l.dealer_org_id
WHERE l.fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR l.status = ANY (sqlc.narg(statuses)::text[])
  )
ORDER BY l.created_at DESC, l.id DESC;

-- name: TransitionFleetDealerLink :one
-- Compare-and-set of the link status: the row changes only while it is
-- still in from_status (no rows: a concurrent change won). active stamps
-- started_at, ended stamps ended_at. ended is final: a new link replaces
-- it.
UPDATE fleet_dealer_links
SET status = sqlc.arg(to_status)::text,
    started_at = CASE WHEN sqlc.arg(to_status)::text = 'active' THEN COALESCE(started_at, NOW()) ELSE started_at END,
    ended_at = CASE WHEN sqlc.arg(to_status)::text = 'ended' THEN NOW() ELSE ended_at END
WHERE uuid = sqlc.arg(uuid) AND status = sqlc.arg(from_status)::text AND status <> 'ended'
RETURNING *;

-- name: SetFleetDealerLinkCari :one
-- Records the fleet cari in the dealer's ledger once (CAS on NULL).
UPDATE fleet_dealer_links
SET cari_account_id = sqlc.arg(cari_account_id)
WHERE id = sqlc.arg(id) AND cari_account_id IS NULL
RETURNING *;

-- name: UpsertFleetReport :one
-- One row per (fleet, period); a rerun of a failed period goes back to
-- pending, a ready report is kept.
INSERT INTO fleet_reports (fleet_org_id, brand_id, period_kind, period_start, period_end, locale)
VALUES (
    sqlc.arg(fleet_org_id), sqlc.arg(brand_id), sqlc.arg(period_kind),
    sqlc.arg(period_start), sqlc.arg(period_end), sqlc.arg(locale)
)
ON CONFLICT (fleet_org_id, period_kind, period_start) DO UPDATE
SET status = CASE WHEN fleet_reports.status = 'failed' THEN 'pending' ELSE fleet_reports.status END,
    error = CASE WHEN fleet_reports.status = 'failed' THEN NULL ELSE fleet_reports.error END,
    locale = CASE WHEN fleet_reports.status = 'failed' THEN EXCLUDED.locale ELSE fleet_reports.locale END
RETURNING *;

-- name: MarkFleetReportReady :one
UPDATE fleet_reports
SET status = 'ready', storage_key = sqlc.arg(storage_key), error = NULL
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: MarkFleetReportFailed :one
UPDATE fleet_reports
SET status = 'failed', error = sqlc.arg(error)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: MarkFleetReportEmailed :one
UPDATE fleet_reports
SET emailed_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'ready' AND emailed_at IS NULL
RETURNING *;

-- name: GetFleetReportByUUID :one
SELECT * FROM fleet_reports WHERE uuid = sqlc.arg(uuid);

-- name: ListFleetReports :many
SELECT * FROM fleet_reports
WHERE fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
ORDER BY period_start DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetReports :one
SELECT COUNT(*)::bigint FROM fleet_reports
WHERE fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  );
