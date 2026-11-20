-- TEC-266 (F2-02a): Glorian sync schema. api_key_enc is always the
-- crypto.SecretBox ciphertext; callers never pass a plain key here.

-- name: CreateIntegrationConnection :one
INSERT INTO integration_connections (
    organization_id, brand_id, key, base_url, api_key_enc,
    default_warehouse_id, active, api_version
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(key), sqlc.arg(base_url),
    sqlc.arg(api_key_enc), sqlc.narg(default_warehouse_id), sqlc.arg(active), sqlc.arg(api_version)
)
RETURNING *;

-- name: GetIntegrationConnectionByUUID :one
SELECT * FROM integration_connections
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetIntegrationConnectionByKey :one
SELECT * FROM integration_connections
WHERE brand_id = sqlc.arg(brand_id) AND key = sqlc.arg(key);

-- name: ListIntegrationConnectionsByKey :many
SELECT * FROM integration_connections
WHERE key = sqlc.arg(key)
ORDER BY id;

-- name: GetIntegrationConnectionForProduct :one
-- The connection of the product's brand with the given key.
SELECT c.* FROM integration_connections c
JOIN products p ON p.brand_id = c.brand_id
WHERE p.uuid = sqlc.arg(product_uuid) AND c.key = sqlc.arg(key);

-- name: ListIntegrationConnections :many
SELECT * FROM integration_connections
WHERE brand_id = sqlc.arg(brand_id)
ORDER BY key;

-- name: UpdateIntegrationConnection :one
UPDATE integration_connections
SET base_url = sqlc.arg(base_url),
    default_warehouse_id = sqlc.narg(default_warehouse_id),
    active = sqlc.arg(active),
    api_version = sqlc.arg(api_version)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetIntegrationConnectionAPIKey :one
-- A key typed in by hand replaces a pending package key too (TEC-1265).
UPDATE integration_connections
SET api_key_enc = sqlc.arg(api_key_enc),
    pending_api_key_enc = '',
    pending_key_issued_at = NULL
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetIntegrationConnectionPackage :one
-- TEC-1265: a new connection package. With rotate the new key waits in
-- pending_api_key_enc until the hub accepts it; without (no key yet) it
-- becomes the active key at once.
UPDATE integration_connections
SET api_key_enc = CASE WHEN sqlc.arg(rotate)::bool THEN api_key_enc ELSE sqlc.arg(api_key_enc)::text END,
    pending_api_key_enc = CASE WHEN sqlc.arg(rotate)::bool THEN sqlc.arg(api_key_enc)::text ELSE '' END,
    pending_key_issued_at = CASE WHEN sqlc.arg(rotate)::bool THEN sqlc.arg(issued_at)::timestamptz ELSE NULL END,
    base_url = sqlc.arg(base_url),
    package_issued_at = sqlc.arg(issued_at)::timestamptz,
    package_expires_at = sqlc.arg(expires_at)::timestamptz
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: PromoteIntegrationConnectionPendingKey :execrows
-- TEC-1265: the hub accepted the pending key. Compare-and-swap on the
-- pending ciphertext so a newer package issued meanwhile is not lost.
UPDATE integration_connections
SET api_key_enc = pending_api_key_enc,
    pending_api_key_enc = '',
    pending_key_issued_at = NULL
WHERE id = sqlc.arg(id)
  AND pending_api_key_enc <> ''
  AND pending_api_key_enc = sqlc.arg(pending_api_key_enc);

-- name: DeleteIntegrationConnection :execrows
DELETE FROM integration_connections
WHERE id = sqlc.arg(id);

-- name: CreateConnectionLocationMap :one
-- TEC-769: remote location name -> warehouse (+ optional bin).
INSERT INTO connection_location_maps (
    organization_id, brand_id, connection_id, warehouse_id, warehouse_location_id, remote_location_code
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id),
    sqlc.arg(warehouse_id), sqlc.narg(warehouse_location_id), sqlc.arg(remote_location_code)
)
RETURNING *;

-- name: UpdateConnectionLocationMap :one
UPDATE connection_location_maps
SET warehouse_id = sqlc.arg(warehouse_id),
    warehouse_location_id = sqlc.narg(warehouse_location_id),
    remote_location_code = sqlc.arg(remote_location_code)
WHERE connection_id = sqlc.arg(connection_id) AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: ListConnectionLocationMaps :many
-- List contract: sort keys remote_location_code, warehouse, created_at (id
-- tiebreak); q on the remote code, warehouse name/code and bin full code.
SELECT m.*, w.uuid AS warehouse_uuid, w.code AS warehouse_code, w.name AS warehouse_name,
       l.uuid AS location_uuid, l.code AS location_code, l.full_code AS location_full_code
FROM connection_location_maps m
JOIN warehouses w ON w.id = m.warehouse_id
LEFT JOIN warehouse_locations l ON l.id = m.warehouse_location_id
WHERE m.connection_id = sqlc.arg(connection_id)
  AND (sqlc.narg(q)::text IS NULL
       OR m.remote_location_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR w.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR w.code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR l.full_code ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'remote_location_code' THEN lower(m.remote_location_code) WHEN 'warehouse' THEN lower(w.name) END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'remote_location_code' THEN lower(m.remote_location_code) WHEN 'warehouse' THEN lower(w.name) END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN m.id END DESC,
  m.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountConnectionLocationMaps :one
SELECT count(*) FROM connection_location_maps m
JOIN warehouses w ON w.id = m.warehouse_id
LEFT JOIN warehouse_locations l ON l.id = m.warehouse_location_id
WHERE m.connection_id = sqlc.arg(connection_id)
  AND (sqlc.narg(q)::text IS NULL
       OR m.remote_location_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR w.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR w.code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR l.full_code ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: GetConnectionLocationMapByRemote :one
SELECT * FROM connection_location_maps
WHERE connection_id = sqlc.arg(connection_id) AND remote_location_code = sqlc.arg(remote_location_code);

-- name: GetConnectionLocationMapByUUID :one
SELECT * FROM connection_location_maps
WHERE connection_id = sqlc.arg(connection_id) AND uuid = sqlc.arg(uuid);

-- name: DeleteConnectionLocationMap :execrows
DELETE FROM connection_location_maps
WHERE connection_id = sqlc.arg(connection_id) AND uuid = sqlc.arg(uuid);

-- name: StartIntegrationSyncRun :one
INSERT INTO integration_sync_runs (organization_id, brand_id, connection_id, kind)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id), sqlc.arg(kind))
RETURNING *;

-- name: FinishIntegrationSyncRun :one
UPDATE integration_sync_runs
SET status = sqlc.arg(status),
    finished_at = NOW(),
    watermark = sqlc.narg(watermark),
    counts = sqlc.arg(counts),
    error = sqlc.narg(error)
WHERE id = sqlc.arg(id) AND status = 'running'
RETURNING *;

-- name: LastSucceededIntegrationSyncRun :one
-- The latest successful run of a kind; its watermark seeds the next
-- incremental pull.
SELECT * FROM integration_sync_runs
WHERE connection_id = sqlc.arg(connection_id) AND kind = sqlc.arg(kind) AND status = 'succeeded'
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: ListIntegrationSyncRuns :many
SELECT * FROM integration_sync_runs
WHERE connection_id = sqlc.arg(connection_id)
ORDER BY started_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: InsertIntegrationExternalParty :one
INSERT INTO integration_external_parties (
    organization_id, brand_id, connection_id, remote_id, name, phone_e164, active, dealer_code
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id), sqlc.arg(remote_id),
    sqlc.arg(name), sqlc.narg(phone_e164), sqlc.arg(active), sqlc.narg(dealer_code)
)
RETURNING *;

-- name: UpsertIntegrationExternalParty :one
INSERT INTO integration_external_parties (
    organization_id, brand_id, connection_id, remote_id, name, phone_e164, active, dealer_code
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id), sqlc.arg(remote_id),
    sqlc.arg(name), sqlc.narg(phone_e164), sqlc.arg(active), sqlc.narg(dealer_code)
)
ON CONFLICT (connection_id, remote_id)
DO UPDATE SET name = EXCLUDED.name,
              phone_e164 = EXCLUDED.phone_e164,
              active = EXCLUDED.active,
              dealer_code = EXCLUDED.dealer_code,
              synced_at = NOW()
RETURNING *;

-- name: GetIntegrationExternalPartyByRemoteID :one
SELECT * FROM integration_external_parties
WHERE connection_id = sqlc.arg(connection_id) AND remote_id = sqlc.arg(remote_id);

-- name: ListIntegrationExternalParties :many
SELECT * FROM integration_external_parties
WHERE connection_id = sqlc.arg(connection_id)
ORDER BY name, id;

-- name: ListGlorianDealerMatches :many
-- TEC-1265 (karar 6): the dealers of a connection with their match
-- (glorian_dealer_matches). List contract: sort name, dealer_code,
-- match_type, synced_at (id tiebreak); q on the dealer name, code, phone
-- and the matched organization name; match_type CSV; active boolean;
-- party_id narrows it to one dealer (the view after a write).
SELECT p.id, p.uuid, p.remote_id, p.name, p.dealer_code, p.phone_e164, p.active, p.synced_at, p.linked_at,
       m.match_type::text AS match_type,
       o.uuid AS org_uuid, o.name AS org_name, o.city AS org_city, o.phone AS org_phone,
       o.glorian_dealer_code AS org_dealer_code
FROM integration_external_parties p
JOIN glorian_dealer_matches m ON m.party_id = p.id
LEFT JOIN organizations o ON o.id = m.organization_id
WHERE p.connection_id = sqlc.arg(connection_id)
  AND (sqlc.narg(q)::text IS NULL
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.dealer_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.phone_e164 ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (cardinality(sqlc.arg(match_types)::text[]) = 0 OR m.match_type = ANY(sqlc.arg(match_types)::text[]))
  AND (sqlc.narg(active)::bool IS NULL OR p.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(party_id)::bigint IS NULL OR p.id = sqlc.narg(party_id)::bigint)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(p.name) WHEN 'dealer_code' THEN lower(p.dealer_code) WHEN 'match_type' THEN m.match_type END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'name' THEN lower(p.name) WHEN 'dealer_code' THEN lower(p.dealer_code) WHEN 'match_type' THEN m.match_type END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'synced_at' THEN p.synced_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'synced_at' THEN p.synced_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN p.id END DESC,
  p.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountGlorianDealerMatches :one
SELECT count(*) FROM integration_external_parties p
JOIN glorian_dealer_matches m ON m.party_id = p.id
LEFT JOIN organizations o ON o.id = m.organization_id
WHERE p.connection_id = sqlc.arg(connection_id)
  AND (sqlc.narg(q)::text IS NULL
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.dealer_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.phone_e164 ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (cardinality(sqlc.arg(match_types)::text[]) = 0 OR m.match_type = ANY(sqlc.arg(match_types)::text[]))
  AND (sqlc.narg(active)::bool IS NULL OR p.active = sqlc.narg(active)::bool);

-- name: GetIntegrationExternalPartyByUUID :one
SELECT * FROM integration_external_parties
WHERE connection_id = sqlc.arg(connection_id) AND uuid = sqlc.arg(uuid);

-- name: SetIntegrationExternalPartyLink :one
-- TEC-1265: manual link of a dealer to an organization (NULL clears it).
UPDATE integration_external_parties
SET linked_organization_id = sqlc.narg(linked_organization_id),
    linked_at = CASE WHEN sqlc.narg(linked_organization_id)::bigint IS NULL THEN NULL ELSE NOW() END
WHERE connection_id = sqlc.arg(connection_id) AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: SearchGlorianDealerOrganizations :many
-- TEC-1265: organization picker of the dealer link (the brand's live
-- organizations, at most row_limit), with their Glorian dealer code.
SELECT o.id, o.uuid, o.name, o.city, o.phone, o.type, o.glorian_dealer_code
FROM organizations o
WHERE o.brand_id = sqlc.arg(brand_id) AND o.deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.city ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.phone ILIKE '%' || sqlc.narg(q)::text || '%'
       OR o.glorian_dealer_code ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY lower(o.name), o.id
LIMIT sqlc.arg(row_limit);

-- name: GetBrandOrganizationByUUID :one
SELECT * FROM organizations
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL;

-- name: SetOrganizationGlorianDealerCode :one
-- TEC-1265: the organization's Glorian dealer code (NULL clears it).
UPDATE organizations
SET glorian_dealer_code = sqlc.narg(glorian_dealer_code)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: CreateOrderOutbound :one
INSERT INTO order_outbounds (
    organization_id, brand_id, order_id, connection_id, external_reference, state, held_reason
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(order_id), sqlc.arg(connection_id),
    sqlc.arg(external_reference), sqlc.arg(state), sqlc.narg(held_reason)
)
RETURNING *;

-- name: GetOrderOutbound :one
SELECT * FROM order_outbounds
WHERE order_id = sqlc.arg(order_id) AND connection_id = sqlc.arg(connection_id);

-- name: LockOrderOutbound :one
SELECT * FROM order_outbounds
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: UpdateOrderOutboundState :one
-- Records one attempt: the new state, the hold reason (NULL unless held)
-- and the last error (NULL on success).
UPDATE order_outbounds
SET state = sqlc.arg(state),
    held_reason = sqlc.narg(held_reason),
    attempts = attempts + sqlc.arg(attempt_increment)::int,
    last_error = sqlc.narg(last_error)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListOrderOutboundsByState :many
SELECT * FROM order_outbounds
WHERE connection_id = sqlc.arg(connection_id) AND state = sqlc.arg(state)
ORDER BY id
LIMIT sqlc.arg(row_limit);

-- TEC-268 (F2-02c): Glorian catalog pull. A synced product is found by its
-- connection + remote id; the pull rewrites only the remote-sourced columns
-- (images, unit type and fixed barcode stay local).

-- name: GetProductByConnectionExternalID :one
SELECT * FROM products
WHERE connection_id = sqlc.arg(connection_id) AND external_id = sqlc.arg(external_id);

-- name: UpdateSyncedProduct :one
UPDATE products
SET category_id = sqlc.arg(category_id),
    sku = sqlc.arg(sku),
    name = sqlc.arg(name),
    description_md = sqlc.arg(description_md),
    warranty_duration_months = sqlc.narg(warranty_duration_months),
    micron_thickness = sqlc.narg(micron_thickness),
    active = sqlc.arg(active),
    external_id = sqlc.arg(external_id),
    connection_id = sqlc.arg(connection_id),
    -- TEC-1251 (K1, 19.11): the meter sale settings (sellable_by_meter,
    -- default_roll_meters, min_cut_meters, product_prices.meter_price) are
    -- local and never rewritten by the pull.
    locked_fields = sqlc.arg(locked_fields)::text[]
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: BrandHasIntegrationConnection :one
-- A brand with an integration connection takes its categories from the
-- remote hub, so their remote-sourced fields are locked in the panel.
SELECT EXISTS (
    SELECT 1 FROM integration_connections WHERE brand_id = sqlc.arg(brand_id)
)::bool;

-- name: SearchWarehouseLocationsForMap :many
-- TEC-769: bin picker of the Glorian location map (active locations of one
-- warehouse, matched on the full code or the name).
SELECT l.* FROM warehouse_locations l
WHERE l.organization_id = sqlc.arg(organization_id) AND l.warehouse_id = sqlc.arg(warehouse_id) AND l.active
  AND (sqlc.narg(q)::text IS NULL
       OR l.full_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR l.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY l.full_code NULLS LAST, l.id
LIMIT sqlc.arg(row_limit);
