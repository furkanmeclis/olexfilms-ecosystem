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
UPDATE integration_connections
SET api_key_enc = sqlc.arg(api_key_enc)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteIntegrationConnection :execrows
DELETE FROM integration_connections
WHERE id = sqlc.arg(id);

-- name: UpsertConnectionLocationMap :one
INSERT INTO connection_location_maps (
    organization_id, brand_id, connection_id, warehouse_location_id, remote_location_code
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id),
    sqlc.arg(warehouse_location_id), sqlc.arg(remote_location_code)
)
ON CONFLICT (connection_id, warehouse_location_id)
DO UPDATE SET remote_location_code = EXCLUDED.remote_location_code
RETURNING *;

-- name: ListConnectionLocationMaps :many
SELECT * FROM connection_location_maps
WHERE connection_id = sqlc.arg(connection_id)
ORDER BY remote_location_code;

-- name: GetConnectionLocationMapByRemote :one
SELECT * FROM connection_location_maps
WHERE connection_id = sqlc.arg(connection_id) AND remote_location_code = sqlc.arg(remote_location_code);

-- name: DeleteConnectionLocationMap :execrows
DELETE FROM connection_location_maps
WHERE connection_id = sqlc.arg(connection_id) AND warehouse_location_id = sqlc.arg(warehouse_location_id);

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
    organization_id, brand_id, connection_id, remote_id, name, phone_e164, active
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id), sqlc.arg(remote_id),
    sqlc.arg(name), sqlc.narg(phone_e164), sqlc.arg(active)
)
RETURNING *;

-- name: UpsertIntegrationExternalParty :one
INSERT INTO integration_external_parties (
    organization_id, brand_id, connection_id, remote_id, name, phone_e164, active
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(connection_id), sqlc.arg(remote_id),
    sqlc.arg(name), sqlc.narg(phone_e164), sqlc.arg(active)
)
ON CONFLICT (connection_id, remote_id)
DO UPDATE SET name = EXCLUDED.name,
              phone_e164 = EXCLUDED.phone_e164,
              active = EXCLUDED.active,
              synced_at = NOW()
RETURNING *;

-- name: GetIntegrationExternalPartyByRemoteID :one
SELECT * FROM integration_external_parties
WHERE connection_id = sqlc.arg(connection_id) AND remote_id = sqlc.arg(remote_id);

-- name: ListIntegrationExternalParties :many
SELECT * FROM integration_external_parties
WHERE connection_id = sqlc.arg(connection_id)
ORDER BY name, id;

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
